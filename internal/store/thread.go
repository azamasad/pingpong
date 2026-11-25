package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Thread is a saved conversation bound to one machine and model (V7.5).
type Thread struct {
	ID             string
	Title          string
	Machine        string
	Model          string
	PinChangesJSON string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s *Store) InsertThread(ctx context.Context, t *Thread) error {
	pin := t.PinChangesJSON
	if pin == "" {
		pin = "[]"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO threads (id, title, machine, model, pin_changes_json, created_at, updated_at)
VALUES (?,?,?,?,?,?,?)`,
		t.ID, t.Title, t.Machine, t.Model, pin,
		t.CreatedAt.UTC().Format(time.RFC3339), t.UpdatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetThread(ctx context.Context, id string) (*Thread, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, title, machine, model, pin_changes_json, created_at, updated_at FROM threads WHERE id=?`, id)
	var t Thread
	var ca, ua string
	if err := row.Scan(&t.ID, &t.Title, &t.Machine, &t.Model, &t.PinChangesJSON, &ca, &ua); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, ca)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
	if t.PinChangesJSON == "" {
		t.PinChangesJSON = "[]"
	}
	return &t, nil
}

func (s *Store) UpdateThread(ctx context.Context, t *Thread) error {
	res, err := s.db.ExecContext(ctx, `UPDATE threads SET title=?, machine=?, model=?, pin_changes_json=?, updated_at=? WHERE id=?`,
		t.Title, t.Machine, t.Model, t.PinChangesJSON, t.UpdatedAt.UTC().Format(time.RFC3339), t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteThread(ctx context.Context, id string) error {
	jobs, err := s.ThreadJobs(ctx, id)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE job_id=?`, j.ID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, j.ID)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM threads WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListThreads(ctx context.Context, before *time.Time, limit int) ([]*Thread, error) {
	q := `SELECT id, title, machine, model, pin_changes_json, created_at, updated_at FROM threads`
	args := []any{}
	if before != nil {
		q += ` WHERE updated_at < ?`
		args = append(args, before.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY updated_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Thread
	for rows.Next() {
		var t Thread
		var ca, ua string
		if err := rows.Scan(&t.ID, &t.Title, &t.Machine, &t.Model, &t.PinChangesJSON, &ca, &ua); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
		if t.PinChangesJSON == "" {
			t.PinChangesJSON = "[]"
		}
		out = append(out, &t)
	}
	return out, nil
}

func (s *Store) ThreadJobs(ctx context.Context, threadID string) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE thread_id=? ORDER BY turn ASC`, threadID)
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

func (s *Store) ThreadActiveJob(ctx context.Context, threadID string) (string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id FROM jobs WHERE thread_id=? AND status IN ('waiting','running','retrying') LIMIT 1`, threadID)
	var id string
	err := row.Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) ThreadTurnCount(ctx context.Context, threadID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE thread_id=?`, threadID).Scan(&n)
	return n, err
}

func (s *Store) MaxThreadTurn(ctx context.Context, threadID string) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT MAX(turn) FROM jobs WHERE thread_id=?`, threadID)
	var max sql.NullInt64
	if err := row.Scan(&max); err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64), nil
}

// PurgeOldThreads deletes threads whose last activity is before cut, with all turn jobs.
func (s *Store) PurgeOldThreads(ctx context.Context, before time.Time) (int64, error) {
	cut := before.UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM threads WHERE updated_at < ?`, cut)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total int64
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return total, err
		}
		if err := s.DeleteThread(ctx, id); err != nil {
			continue
		}
		total++
	}
	return total, nil
}