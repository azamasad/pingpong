package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FailureSummary is a recent failed or retrying job without content.
type FailureSummary struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
	ErrorCode string    `json:"error_code"`
	Machine   string    `json:"machine"`
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) DBPath() string { return filepath.Join(s.dir, "pingpong.db") }

func (s *Store) DBSizeBytes() (int64, error) {
	fi, err := os.Stat(s.DBPath())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return fi.Size(), nil
}

// JobCounts returns counts grouped by status.
func (s *Store) JobCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// LaneCounts returns counts grouped by lane for active-ish statuses.
func (s *Store) LaneCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT lane, COUNT(*) FROM jobs
WHERE status IN ('waiting','running','retrying') GROUP BY lane`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var lane string
		var n int
		if err := rows.Scan(&lane, &n); err != nil {
			return nil, err
		}
		out[lane] = n
	}
	return out, rows.Err()
}

// OldestWaitingSeconds is the age of the oldest waiting job, or nil if none.
func (s *Store) OldestWaitingSeconds(ctx context.Context) (*float64, error) {
	var created string
	err := s.db.QueryRowContext(ctx, `
SELECT created_at FROM jobs WHERE status='waiting' ORDER BY created_at ASC LIMIT 1`).Scan(&created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, err
	}
	sec := time.Since(t).Seconds()
	return &sec, nil
}

// RecentFailures lists recent failed or retrying jobs without prompts or outputs.
func (s *Store) RecentFailures(ctx context.Context, limit int) ([]FailureSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, updated_at, COALESCE(error_code,''), COALESCE(machine,'')
FROM jobs WHERE status IN ('failed','retrying')
ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FailureSummary
	for rows.Next() {
		var f FailureSummary
		var updated string
		if err := rows.Scan(&f.ID, &updated, &f.ErrorCode, &f.Machine); err != nil {
			return nil, err
		}
		f.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, f)
	}
	return out, rows.Err()
}

// BackupTo writes a consistent copy to destPath using VACUUM INTO.
func BackupTo(srcDir, destPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("destination already exists")
	}
	db, err := sql.Open("sqlite", filepath.Join(srcDir, "pingpong.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf("VACUUM INTO %q", destPath)); err != nil {
		return err
	}
	return os.Chmod(destPath, 0600)
}
