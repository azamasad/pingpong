package store

import (
	"context"
	"database/sql"
)

const (
	StageAnswer = "answer"
)

func (s *Store) ClearCheckPending(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET check_pending=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) ClearCheckPendingTx(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, `UPDATE jobs SET check_pending=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) SaveTokenStats(ctx context.Context, id string, promptTok, outputTok int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET prompt_tokens=?, output_tokens=? WHERE id=?`,
		promptTok, outputTok, id)
	return err
}

func (s *Store) JobsNeedingCheckStage(ctx context.Context) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs
WHERE check_pending=1 AND status='done' AND stage=?`, StageAnswer)
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

func (s *Store) ChildByParentStage(ctx context.Context, parentID, stage string) (*Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE parent_id=? AND stage=? LIMIT 1`,
		parentID, stage)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return j, err
}

func (s *Store) ChildByParentStageTx(ctx context.Context, tx *sql.Tx, parentID, stage string) (*Job, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE parent_id=? AND stage=? LIMIT 1`,
		parentID, stage)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return j, err
}

func (s *Store) ChildrenOf(ctx context.Context, parentID string) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE parent_id=? ORDER BY created_at ASC`, parentID)
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

// CheckBatchCounts tallies check stages for a batch.
func (s *Store) CheckBatchCounts(ctx context.Context, batchID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, stage, check_pending FROM jobs WHERE batch_id=?`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{"pending": 0, "done": 0, "failed": 0}
	for rows.Next() {
		var st, stage string
		var cp int
		if err := rows.Scan(&st, &stage, &cp); err != nil {
			return nil, err
		}
		if stage == StageAnswer && cp == 1 {
			out["pending"]++
			continue
		}
		if stage != "review" && stage != "revise" {
			continue
		}
		switch st {
		case "waiting", "running", "retrying":
			out["pending"]++
		case "done":
			out["done"]++
		case "failed", "cancelled":
			out["failed"]++
		}
	}
	return out, nil
}

func (s *Store) BatchPendingWithChecks(ctx context.Context, batchID string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, stage, check_pending FROM jobs WHERE batch_id=?`, batchID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var st, stage string
		var cp int
		if err := rows.Scan(&st, &stage, &cp); err != nil {
			return 0, err
		}
		if stage != "" && stage != StageAnswer {
			switch st {
			case "waiting", "running", "retrying":
				n++
			}
			continue
		}
		if cp == 1 && st == "done" {
			n++
			continue
		}
		switch st {
		case "waiting", "running", "retrying":
			if stage == "" || stage == StageAnswer {
				n++
			}
		}
	}
	return n, nil
}