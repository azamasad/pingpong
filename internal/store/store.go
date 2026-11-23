package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type JobStatus string

const (
	StatusWaiting   JobStatus = "waiting"
	StatusRunning   JobStatus = "running"
	StatusRetrying  JobStatus = "retrying"
	StatusDone      JobStatus = "done"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

type Job struct {
	ID             string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Status         JobStatus
	RequestJSON    string
	Machine        string
	Model          string
	Mode           string
	OutputText     string
	OutputThinking string
	ErrorCode      string
	ErrorMessage   string
	Attempt        int
	FirstFailureAt *time.Time
	NextAttemptAt  *time.Time
}

type Event struct {
	ID        int64
	JobID     string
	Seq       int64
	CreatedAt time.Time
	Type      string
	DataJSON  string
}

type Store struct {
	db *sql.DB
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "pingpong.db")
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  status TEXT NOT NULL,
  request_json TEXT NOT NULL,
  machine TEXT,
  model TEXT,
  mode TEXT,
  output_text TEXT,
  output_thinking TEXT,
  error_code TEXT,
  error_message TEXT,
  attempt INTEGER NOT NULL DEFAULT 0,
  first_failure_at TEXT,
  next_attempt_at TEXT
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  type TEXT NOT NULL,
  data_json TEXT,
  UNIQUE(job_id, seq),
  FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_events_job ON events(job_id, seq);
`)
	return err
}

func (s *Store) InsertJob(ctx context.Context, j *Job) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO jobs (
id, created_at, updated_at, status, request_json, machine, model, mode, attempt
) VALUES (?,?,?,?,?,?,?,?,?)`,
		j.ID, j.CreatedAt.UTC().Format(time.RFC3339), j.UpdatedAt.UTC().Format(time.RFC3339),
		j.Status, j.RequestJSON, j.Machine, j.Model, j.Mode, j.Attempt)
	return err
}

func (s *Store) UpdateJob(ctx context.Context, j *Job) error {
	ff, na := "", ""
	if j.FirstFailureAt != nil {
		ff = j.FirstFailureAt.UTC().Format(time.RFC3339)
	}
	if j.NextAttemptAt != nil {
		na = j.NextAttemptAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET updated_at=?, status=?, machine=?, model=?, mode=?,
output_text=?, output_thinking=?, error_code=?, error_message=?, attempt=?, first_failure_at=?, next_attempt_at=?
WHERE id=?`,
		j.UpdatedAt.UTC().Format(time.RFC3339), j.Status, j.Machine, j.Model, j.Mode,
		j.OutputText, j.OutputThinking, j.ErrorCode, j.ErrorMessage, j.Attempt, ff, na, j.ID)
	return err
}

func (s *Store) GetJob(ctx context.Context, id string) (*Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, created_at, updated_at, status, request_json, machine, model, mode,
output_text, output_thinking, error_code, error_message, attempt, first_failure_at, next_attempt_at FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

func scanJob(row *sql.Row) (*Job, error) {
	var j Job
	var ca, ua, ff, na sql.NullString
	var out, think, ec, em sql.NullString
	if err := row.Scan(&j.ID, &ca, &ua, &j.Status, &j.RequestJSON, &j.Machine, &j.Model, &j.Mode,
		&out, &think, &ec, &em, &j.Attempt, &ff, &na); err != nil {
		return nil, err
	}
	j.OutputText, j.OutputThinking = out.String, think.String
	j.ErrorCode, j.ErrorMessage = ec.String, em.String
	j.CreatedAt, _ = time.Parse(time.RFC3339, ca.String)
	j.UpdatedAt, _ = time.Parse(time.RFC3339, ua.String)
	if ff.Valid {
		t, _ := time.Parse(time.RFC3339, ff.String)
		j.FirstFailureAt = &t
	}
	if na.Valid {
		t, _ := time.Parse(time.RFC3339, na.String)
		j.NextAttemptAt = &t
	}
	return &j, nil
}

func (s *Store) ListJobs(ctx context.Context, status string, limit int) ([]*Job, error) {
	q := `SELECT id, created_at, updated_at, status, request_json, machine, model, mode,
output_text, output_thinking, error_code, error_message, attempt, first_failure_at, next_attempt_at FROM jobs`
	args := []any{}
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
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

func scanJobRow(rows *sql.Rows) (*Job, error) {
	var j Job
	var ca, ua, ff, na sql.NullString
	var out, think, ec, em sql.NullString
	if err := rows.Scan(&j.ID, &ca, &ua, &j.Status, &j.RequestJSON, &j.Machine, &j.Model, &j.Mode,
		&out, &think, &ec, &em, &j.Attempt, &ff, &na); err != nil {
		return nil, err
	}
	j.OutputText, j.OutputThinking = out.String, think.String
	j.ErrorCode, j.ErrorMessage = ec.String, em.String
	j.CreatedAt, _ = time.Parse(time.RFC3339, ca.String)
	j.UpdatedAt, _ = time.Parse(time.RFC3339, ua.String)
	if ff.Valid {
		t, _ := time.Parse(time.RFC3339, ff.String)
		j.FirstFailureAt = &t
	}
	if na.Valid {
		t, _ := time.Parse(time.RFC3339, na.String)
		j.NextAttemptAt = &t
	}
	return &j, nil
}

func (s *Store) DeleteJob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE job_id=?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, id)
	return err
}

func (s *Store) NextEventSeq(ctx context.Context, jobID string) (int64, error) {
	var m sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM events WHERE job_id=?`, jobID).Scan(&m)
	if err != nil {
		return 0, err
	}
	if !m.Valid {
		return 1, nil
	}
	return m.Int64 + 1, nil
}

func (s *Store) AppendEvent(ctx context.Context, jobID string, seq int64, typ string, data any) error {
	b, _ := json.Marshal(data)
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (job_id, seq, created_at, type, data_json) VALUES (?,?,?,?,?)`,
		jobID, seq, time.Now().UTC().Format(time.RFC3339), typ, string(b))
	return err
}

func (s *Store) EventsSince(ctx context.Context, jobID string, fromSeq int64) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, job_id, seq, created_at, type, data_json FROM events WHERE job_id=? AND seq >= ? ORDER BY seq`,
		jobID, fromSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ca string
		if err := rows.Scan(&e.ID, &e.JobID, &e.Seq, &ca, &e.Type, &e.DataJSON); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) ActiveJobs(ctx context.Context) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, updated_at, status, request_json, machine, model, mode,
output_text, output_thinking, error_code, error_message, attempt, first_failure_at, next_attempt_at FROM jobs
WHERE status IN ('waiting','running','retrying')`)
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

func (s *Store) PurgeOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE status IN ('done','failed','cancelled') AND updated_at < ?`,
		before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
