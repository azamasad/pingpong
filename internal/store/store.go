package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound means the row does not exist.
var ErrNotFound = errors.New("not found")

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
	Lane           string
	BatchID        string
	ParentID       string
	Stage          string
	CheckPending   int
	PromptTokens   int
	OutputTokens   int
	ThreadID       string
	Turn           int
}

const (
	LaneInteractive = "interactive"
	LaneBatch       = "batch"
)

const jobCols = `id, created_at, updated_at, status, request_json, machine, model, mode,
output_text, output_thinking, error_code, error_message, attempt, first_failure_at, next_attempt_at,
lane, batch_id, parent_id, stage, check_pending, prompt_tokens, output_tokens, thread_id, turn`

type Event struct {
	ID        int64
	JobID     string
	Seq       int64
	CreatedAt time.Time
	Type      string
	DataJSON  string
}

type Store struct {
	db  *sql.DB
	dir string
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
	s := &Store{db: db, dir: dir}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Checkpoint flushes the WAL before exit.
func (s *Store) Checkpoint() error {
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

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
	if err != nil {
		return err
	}
	if err := s.addColumn("jobs", "lane", `ALTER TABLE jobs ADD COLUMN lane TEXT NOT NULL DEFAULT 'interactive'`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "batch_id", `ALTER TABLE jobs ADD COLUMN batch_id TEXT`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "parent_id", `ALTER TABLE jobs ADD COLUMN parent_id TEXT`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "stage", `ALTER TABLE jobs ADD COLUMN stage TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "check_pending", `ALTER TABLE jobs ADD COLUMN check_pending INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "prompt_tokens", `ALTER TABLE jobs ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "output_tokens", `ALTER TABLE jobs ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS batches (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  kind TEXT NOT NULL,
  item_count INTEGER NOT NULL,
  plan_json TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_jobs_batch ON jobs(batch_id);
CREATE INDEX IF NOT EXISTS idx_jobs_parent ON jobs(parent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_parent_stage ON jobs(parent_id, stage)
  WHERE parent_id IS NOT NULL AND parent_id != '';
CREATE TABLE IF NOT EXISTS templates (
  name TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  defaults_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS threads (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  machine TEXT NOT NULL,
  model TEXT NOT NULL,
  pin_changes_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	if err := s.addColumn("jobs", "thread_id", `ALTER TABLE jobs ADD COLUMN thread_id TEXT`); err != nil {
		return err
	}
	if err := s.addColumn("jobs", "turn", `ALTER TABLE jobs ADD COLUMN turn INTEGER`); err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_thread ON jobs(thread_id, turn)`)
	return err
}

func (s *Store) addColumn(table, name, stmt string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var col, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &col, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if col == name {
			return nil
		}
	}
	_, err = s.db.Exec(stmt)
	return err
}

func (s *Store) InsertJob(ctx context.Context, j *Job) error {
	lane := j.Lane
	if lane == "" {
		lane = LaneInteractive
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO jobs (
id, created_at, updated_at, status, request_json, machine, model, mode, attempt, lane, batch_id,
parent_id, stage, check_pending, prompt_tokens, output_tokens, thread_id, turn
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.CreatedAt.UTC().Format(time.RFC3339), j.UpdatedAt.UTC().Format(time.RFC3339),
		j.Status, j.RequestJSON, j.Machine, j.Model, j.Mode, j.Attempt, lane, j.BatchID,
		j.ParentID, j.Stage, j.CheckPending, j.PromptTokens, j.OutputTokens, j.ThreadID, j.Turn)
	return err
}

// SaveOutput writes generated text without touching status. A flush must not
// put a cancelled or finished job back to running.
func (s *Store) SaveOutput(ctx context.Context, id, text, thinking string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET output_text=?, output_thinking=?, updated_at=? WHERE id=?`,
		text, thinking, at.UTC().Format(time.RFC3339), id)
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
	row := s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

func scanJob(row *sql.Row) (*Job, error) {
	var j Job
	var ca, ua, ff, na sql.NullString
	var out, think, ec, em, lane, batch, parent sql.NullString
	var stage sql.NullString
	var threadID sql.NullString
	var turn sql.NullInt64
	if err := row.Scan(&j.ID, &ca, &ua, &j.Status, &j.RequestJSON, &j.Machine, &j.Model, &j.Mode,
		&out, &think, &ec, &em, &j.Attempt, &ff, &na, &lane, &batch,
		&parent, &stage, &j.CheckPending, &j.PromptTokens, &j.OutputTokens, &threadID, &turn); err != nil {
		return nil, err
	}
	j.OutputText, j.OutputThinking = out.String, think.String
	j.ErrorCode, j.ErrorMessage = ec.String, em.String
	j.Lane, j.BatchID = lane.String, batch.String
	j.ParentID, j.Stage = parent.String, stage.String
	j.ThreadID = threadID.String
	if turn.Valid {
		j.Turn = int(turn.Int64)
	}
	if j.Lane == "" {
		j.Lane = LaneInteractive
	}
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

func (s *Store) ListJobs(ctx context.Context, status, lane, batchID string, before *time.Time, limit int) ([]*Job, error) {
	q := `SELECT ` + jobCols + ` FROM jobs`
	args := []any{}
	where := []string{}
	if status != "" {
		where = append(where, `status=?`)
		args = append(args, status)
	}
	if lane != "" {
		where = append(where, `lane=?`)
		args = append(args, lane)
	}
	if batchID != "" {
		where = append(where, `batch_id=?`)
		args = append(args, batchID)
	}
	if before != nil {
		where = append(where, `created_at < ?`)
		args = append(args, before.UTC().Format(time.RFC3339))
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
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
	var out, think, ec, em, lane, batch, parent sql.NullString
	var stage sql.NullString
	var threadID sql.NullString
	var turn sql.NullInt64
	if err := rows.Scan(&j.ID, &ca, &ua, &j.Status, &j.RequestJSON, &j.Machine, &j.Model, &j.Mode,
		&out, &think, &ec, &em, &j.Attempt, &ff, &na, &lane, &batch,
		&parent, &stage, &j.CheckPending, &j.PromptTokens, &j.OutputTokens, &threadID, &turn); err != nil {
		return nil, err
	}
	j.OutputText, j.OutputThinking = out.String, think.String
	j.ErrorCode, j.ErrorMessage = ec.String, em.String
	j.Lane, j.BatchID = lane.String, batch.String
	j.ParentID, j.Stage = parent.String, stage.String
	j.ThreadID = threadID.String
	if turn.Valid {
		j.Turn = int(turn.Int64)
	}
	if j.Lane == "" {
		j.Lane = LaneInteractive
	}
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

func (s *Store) WaitingForMachine(ctx context.Context, machine string) ([]*Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE machine=? AND status=? ORDER BY created_at`,
		machine, StatusWaiting)
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

func (s *Store) DeleteJob(ctx context.Context, id string) error {
	children, _ := s.ChildrenOf(ctx, id)
	for _, c := range children {
		if err := s.DeleteJob(ctx, c.ID); err != nil && err != sql.ErrNoRows {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE job_id=?`, id)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
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
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs
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
	cut := before.UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM jobs WHERE (parent_id IS NULL OR parent_id='')
AND (thread_id IS NULL OR thread_id='')
AND status IN ('done','failed','cancelled') AND updated_at < ?`, cut)
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
		childRows, err := s.db.QueryContext(ctx, `SELECT id, status, updated_at FROM jobs WHERE parent_id=?`, id)
		if err != nil {
			continue
		}
		skip := false
		for childRows.Next() {
			var cid, st, ua string
			_ = childRows.Scan(&cid, &st, &ua)
			if st != string(StatusDone) && st != string(StatusFailed) && st != string(StatusCancelled) {
				skip = true
				break
			}
			if ua >= cut {
				skip = true
				break
			}
		}
		childRows.Close()
		if skip {
			continue
		}
		children, _ := s.ChildrenOf(ctx, id)
		for _, c := range children {
			_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE job_id=?`, c.ID)
			if res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, c.ID); err == nil {
				n, _ := res.RowsAffected()
				total += n
			}
		}
		_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE job_id=?`, id)
		if res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, id); err == nil {
			n, _ := res.RowsAffected()
			total += n
		}
	}
	// Sweep standalone jobs (no parent link) and no children pending.
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE status IN ('done','failed','cancelled') AND updated_at < ?
AND (parent_id IS NULL OR parent_id='') AND (thread_id IS NULL OR thread_id='')
AND id NOT IN (SELECT parent_id FROM jobs WHERE parent_id IS NOT NULL AND parent_id != '')`,
		cut)
	if err != nil {
		return total, err
	}
	n, _ := res.RowsAffected()
	total += n
	_, _ = s.PurgeEmptyBatches(ctx)
	return total, nil
}
