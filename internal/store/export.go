package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// JobIndex returns 0-based batch item index per job id (roots only).
type JobIndex struct {
	ByID     map[string]int
	ParentOf map[string]string
}

// BatchItemIndex maps each job in a batch to its export index.
func (s *Store) BatchItemIndex(ctx context.Context, batchID string) (*JobIndex, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, parent_id, stage FROM jobs WHERE batch_id=? ORDER BY created_at ASC, id ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idx := &JobIndex{ByID: map[string]int{}, ParentOf: map[string]string{}}
	next := 0
	for rows.Next() {
		var id, stage string
		var parent sql.NullString
		if err := rows.Scan(&id, &parent, &stage); err != nil {
			return nil, err
		}
		par := parent.String
		idx.ParentOf[id] = par
		if par != "" {
			continue
		}
		if stage == "reduce" {
			continue
		}
		idx.ByID[id] = next
		next++
	}
	for id, parent := range idx.ParentOf {
		if parent == "" {
			continue
		}
		if v, ok := idx.ByID[parent]; ok {
			idx.ByID[id] = v
		}
	}
	return idx, nil
}

// ExportBatchJobs opens a row cursor for batch export (reduce rows last).
func (s *Store) ExportBatchJobs(ctx context.Context, batchID string) (*sql.Rows, error) {
	q := `SELECT ` + jobCols + ` FROM jobs WHERE batch_id=? ORDER BY
CASE WHEN stage='reduce' THEN 1 ELSE 0 END, created_at ASC, id ASC`
	return s.db.QueryContext(ctx, q, batchID)
}

// CountExistingJobIDs returns how many of the ids exist in the store.
func (s *Store) CountExistingJobIDs(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	var n int
	q := `SELECT COUNT(*) FROM jobs WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// ExportJobsByIDs streams jobs matching ids, ordered by created_at, id.
func (s *Store) ExportJobsByIDs(ctx context.Context, ids []string) (*sql.Rows, int, error) {
	if len(ids) == 0 {
		return nil, 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT ` + jobCols + ` FROM jobs WHERE id IN (` + strings.Join(placeholders, ",") +
		`) ORDER BY created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	var n int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...).Scan(&n)
	if err != nil {
		rows.Close()
		return nil, 0, err
	}
	skipped := len(ids) - n
	return rows, skipped, nil
}

// DoneEventStats reads token stats from the last done event.
func (s *Store) DoneEventStats(ctx context.Context, jobID string) (outputTokens int, tps float64, ok bool) {
	var data string
	err := s.db.QueryRowContext(ctx,
		`SELECT data_json FROM events WHERE job_id=? AND type='done' ORDER BY seq DESC LIMIT 1`, jobID).Scan(&data)
	if err != nil {
		return 0, 0, false
	}
	var ev map[string]any
	if json.Unmarshal([]byte(data), &ev) != nil {
		return 0, 0, false
	}
	stats, _ := ev["stats"].(map[string]any)
	if stats == nil {
		return 0, 0, false
	}
	if v, ok := stats["output_tokens"].(float64); ok {
		outputTokens = int(v)
	}
	if v, ok := stats["output_tokens"].(json.Number); ok {
		if i, err := v.Int64(); err == nil {
			outputTokens = int(i)
		}
	}
	if v, ok := stats["output_tokens_per_s"].(float64); ok {
		tps = v
	}
	return outputTokens, tps, outputTokens > 0 || tps > 0
}

// ScanJobRow wraps scanJobRow for external packages in the same module.
func ScanJobRow(rows *sql.Rows) (*Job, error) {
	return scanJobRow(rows)
}

// BatchItemIndexForJob resolves export index using a per-batch cache.
func (s *Store) BatchItemIndexForJob(ctx context.Context, batchID, jobID, parentID string, cache map[string]*JobIndex) (index int, has bool) {
	if batchID == "" {
		return 0, false
	}
	ji, ok := cache[batchID]
	if !ok {
		var err error
		ji, err = s.BatchItemIndex(ctx, batchID)
		if err != nil {
			return 0, false
		}
		cache[batchID] = ji
	}
	if v, ok := ji.ByID[jobID]; ok {
		return v, true
	}
	_ = parentID
	return 0, false
}
