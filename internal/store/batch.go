package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const batchCols = `id, created_at, kind, item_count, plan_json, source`

// Batch is one submitted batch row (V7.1).
type Batch struct {
	ID        string
	CreatedAt time.Time
	Kind      string
	ItemCount int
	PlanJSON  string
	Source    string
}

func (s *Store) InsertBatch(ctx context.Context, b *Batch) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO batches (id, created_at, kind, item_count, plan_json, source)
VALUES (?,?,?,?,?,?)`,
		b.ID, b.CreatedAt.UTC().Format(time.RFC3339), b.Kind, b.ItemCount, b.PlanJSON, b.Source)
	return err
}

func (s *Store) InsertBatchTx(ctx context.Context, tx *sql.Tx, b *Batch) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO batches (id, created_at, kind, item_count, plan_json, source)
VALUES (?,?,?,?,?,?)`,
		b.ID, b.CreatedAt.UTC().Format(time.RFC3339), b.Kind, b.ItemCount, b.PlanJSON, b.Source)
	return err
}

func (s *Store) GetBatch(ctx context.Context, id string) (*Batch, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+batchCols+` FROM batches WHERE id=?`, id)
	return scanBatch(row)
}

func scanBatch(row *sql.Row) (*Batch, error) {
	var b Batch
	var ca string
	var plan, src sql.NullString
	if err := row.Scan(&b.ID, &ca, &b.Kind, &b.ItemCount, &plan, &src); err != nil {
		return nil, err
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339, ca)
	b.PlanJSON, b.Source = plan.String, src.String
	return &b, nil
}

func (s *Store) ListBatches(ctx context.Context, before *time.Time, limit int) ([]*Batch, error) {
	q := `SELECT ` + batchCols + ` FROM batches`
	args := []any{}
	if before != nil {
		q += ` WHERE created_at < ?`
		args = append(args, before.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY created_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Batch
	for rows.Next() {
		var b Batch
		var ca string
		var plan, src sql.NullString
		if err := rows.Scan(&b.ID, &ca, &b.Kind, &b.ItemCount, &plan, &src); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		b.PlanJSON, b.Source = plan.String, src.String
		out = append(out, &b)
	}
	return out, nil
}

// ListBatchJobs returns jobs of a batch in submission order.
func (s *Store) ListBatchJobs(ctx context.Context, batchID string, offset, limit int) ([]*Job, error) {
	q := `SELECT ` + jobCols + ` FROM jobs WHERE batch_id=? ORDER BY created_at ASC, id ASC`
	args := []any{batchID}
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, offset)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (s *Store) CountBatchJobs(ctx context.Context, batchID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE batch_id=?`, batchID).Scan(&n)
	return n, err
}

func (s *Store) BatchJobIDsExist(ctx context.Context, batchID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE batch_id=? LIMIT 1`, batchID).Scan(&n)
	return n > 0, err
}

func (s *Store) InsertJobTx(ctx context.Context, tx *sql.Tx, j *Job) error {
	lane := j.Lane
	if lane == "" {
		lane = LaneInteractive
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO jobs (
id, created_at, updated_at, status, request_json, machine, model, mode, attempt, lane, batch_id,
parent_id, stage, check_pending, prompt_tokens, output_tokens
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.CreatedAt.UTC().Format(time.RFC3339), j.UpdatedAt.UTC().Format(time.RFC3339),
		j.Status, j.RequestJSON, j.Machine, j.Model, j.Mode, j.Attempt, lane, j.BatchID,
		j.ParentID, j.Stage, j.CheckPending, j.PromptTokens, j.OutputTokens)
	return err
}

func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) ActiveJobsByBatch(ctx context.Context, batchID string) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs
WHERE batch_id=? AND status IN ('waiting','running','retrying') ORDER BY created_at ASC`,
		batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (s *Store) PurgeEmptyBatches(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM batches WHERE id NOT IN (SELECT DISTINCT batch_id FROM jobs WHERE batch_id IS NOT NULL AND batch_id != '')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchCounts aggregates status counts for every job in a batch.
func (s *Store) BatchCounts(ctx context.Context, batchID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status FROM jobs WHERE batch_id=?`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := map[string]int{
		"waiting": 0, "running": 0, "retrying": 0,
		"done": 0, "failed": 0, "cancelled": 0,
	}
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return nil, err
		}
		c[st]++
	}
	return c, nil
}

func PendingFromCounts(c map[string]int) int {
	return c["waiting"] + c["running"] + c["retrying"]
}
