package store

import (
	"context"
	"database/sql"
	"time"
)

// SetBatchReduceJobID records the reduce stage job on a batch.
func (s *Store) SetBatchReduceJobID(ctx context.Context, batchID, jobID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE batches SET reduce_job_id=? WHERE id=? AND (reduce_job_id='' OR reduce_job_id IS NULL)`,
		jobID, batchID)
	return err
}

// ClearBatchPendingReduce drops reduce_json so no reduce stage is created later.
func (s *Store) ClearBatchPendingReduce(ctx context.Context, batchID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE batches SET reduce_json='' WHERE id=?`, batchID)
	return err
}

// BatchesNeedingReduce lists batches with reduce_json set and no reduce job yet.
func (s *Store) BatchesNeedingReduce(ctx context.Context) ([]*Batch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+batchCols+` FROM batches
WHERE reduce_json!='' AND (reduce_job_id='' OR reduce_job_id IS NULL)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Batch
	for rows.Next() {
		var b Batch
		var ca string
		var plan, src, rj, rjid sql.NullString
		if err := rows.Scan(&b.ID, &ca, &b.Kind, &b.ItemCount, &plan, &src, &rj, &rjid); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		b.PlanJSON, b.Source = plan.String, src.String
		b.ReduceJSON, b.ReduceJobID = rj.String, rjid.String
		out = append(out, &b)
	}
	return out, nil
}

// BatchChunkJobsTerminal reports whether every non-reduce chunk job in the batch is terminal.
func (s *Store) BatchChunkJobsTerminal(ctx context.Context, batchID string) (bool, int, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, stage, parent_id FROM jobs WHERE batch_id=?
AND (parent_id IS NULL OR parent_id='') AND stage!=?`, batchID, StageReduce)
	if err != nil {
		return false, 0, false, err
	}
	defer rows.Close()
	done := 0
	for rows.Next() {
		var st, stage string
		var parent sql.NullString
		if err := rows.Scan(&st, &stage, &parent); err != nil {
			return false, 0, false, err
		}
		switch st {
		case "waiting", "running", "retrying":
			return false, done, false, nil
		case "done":
			done++
		}
	}
	return true, done, done > 0, nil
}

// ListBatchChunkJobs returns chunk item jobs in submission order (no reduce or stage children).
func (s *Store) ListBatchChunkJobs(ctx context.Context, batchID string) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE batch_id=?
AND (stage='' OR stage=?) AND (parent_id IS NULL OR parent_id='')
ORDER BY created_at ASC, id ASC`, batchID, StageAnswer)
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

// ReduceJobByBatch returns the reduce stage job when present.
func (s *Store) ReduceJobByBatch(ctx context.Context, batchID string) (*Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE batch_id=? AND stage=? LIMIT 1`,
		batchID, StageReduce)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return j, err
}
