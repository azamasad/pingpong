package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Schedule is a saved timed job definition (V7.7).
type Schedule struct {
	ID        string
	Name      string
	RuleJSON  string
	Template  string
	Prompt    string
	VarsJSON  string
	RowsJSON  string
	Machine   string
	Model     string
	Missed    string
	Overlap   string
	Enabled   bool
	LastDue   *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

const scheduleCols = `id, name, rule_json, template, prompt, vars_json, rows_json, machine, model,
missed, overlap, enabled, last_due, created_at, updated_at`

type ScheduleRun struct {
	ID         string
	ScheduleID string
	Due        time.Time
	Started    time.Time
	Kind       string
	JobID      string
	BatchID    string
	Status     string
	ErrorCode  string
}

func scanSchedule(row *sql.Row) (*Schedule, error) {
	var s Schedule
	var ca, ua, ld sql.NullString
	var tmpl, prompt, vars, rows sql.NullString
	var en int
	if err := row.Scan(&s.ID, &s.Name, &s.RuleJSON, &tmpl, &prompt, &vars, &rows,
		&s.Machine, &s.Model, &s.Missed, &s.Overlap, &en, &ld, &ca, &ua); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s.Template, s.Prompt = tmpl.String, prompt.String
	s.VarsJSON, s.RowsJSON = vars.String, rows.String
	s.Enabled = en != 0
	s.CreatedAt, _ = time.Parse(time.RFC3339, ca.String)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, ua.String)
	if ld.Valid {
		t, _ := time.Parse(time.RFC3339, ld.String)
		s.LastDue = &t
	}
	return &s, nil
}

func scanScheduleRow(rows *sql.Rows) (*Schedule, error) {
	var s Schedule
	var ca, ua, ld sql.NullString
	var tmpl, prompt, vars, rowsJSON sql.NullString
	var en int
	if err := rows.Scan(&s.ID, &s.Name, &s.RuleJSON, &tmpl, &prompt, &vars, &rowsJSON,
		&s.Machine, &s.Model, &s.Missed, &s.Overlap, &en, &ld, &ca, &ua); err != nil {
		return nil, err
	}
	s.Template, s.Prompt = tmpl.String, prompt.String
	s.VarsJSON, s.RowsJSON = vars.String, rowsJSON.String
	s.Enabled = en != 0
	s.CreatedAt, _ = time.Parse(time.RFC3339, ca.String)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, ua.String)
	if ld.Valid {
		t, _ := time.Parse(time.RFC3339, ld.String)
		s.LastDue = &t
	}
	return &s, nil
}

func (s *Store) InsertSchedule(ctx context.Context, sch *Schedule) error {
	if sch.ID == "" {
		sch.ID = uuid.NewString()
	}
	ld := ""
	if sch.LastDue != nil {
		ld = sch.LastDue.UTC().Format(time.RFC3339)
	}
	en := 0
	if sch.Enabled {
		en = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO schedules (`+scheduleCols+`)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sch.ID, sch.Name, sch.RuleJSON, nullStr(sch.Template), nullStr(sch.Prompt),
		nullStr(sch.VarsJSON), nullStr(sch.RowsJSON), sch.Machine, sch.Model,
		sch.Missed, sch.Overlap, en, nullStr(ld),
		sch.CreatedAt.UTC().Format(time.RFC3339), sch.UpdatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+scheduleCols+` FROM schedules WHERE id=?`, id)
	return scanSchedule(row)
}

func (s *Store) GetScheduleByName(ctx context.Context, name string) (*Schedule, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+scheduleCols+` FROM schedules WHERE name=?`, name)
	return scanSchedule(row)
}

func (s *Store) ListSchedules(ctx context.Context) ([]*Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+scheduleCols+` FROM schedules ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Schedule
	for rows.Next() {
		sch, err := scanScheduleRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, nil
}

func (s *Store) UpdateSchedule(ctx context.Context, sch *Schedule) error {
	ld := ""
	if sch.LastDue != nil {
		ld = sch.LastDue.UTC().Format(time.RFC3339)
	}
	en := 0
	if sch.Enabled {
		en = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE schedules SET rule_json=?, template=?, prompt=?, vars_json=?, rows_json=?,
machine=?, model=?, missed=?, overlap=?, enabled=?, last_due=?, updated_at=? WHERE id=?`,
		sch.RuleJSON, nullStr(sch.Template), nullStr(sch.Prompt), nullStr(sch.VarsJSON), nullStr(sch.RowsJSON),
		sch.Machine, sch.Model, sch.Missed, sch.Overlap, en, nullStr(ld),
		sch.UpdatedAt.UTC().Format(time.RFC3339), sch.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSchedule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM schedule_runs WHERE schedule_id=?`, id)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountSchedules(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedules`).Scan(&n)
	return n, err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) InsertScheduleRun(ctx context.Context, run *ScheduleRun) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO schedule_runs
(id, schedule_id, due, started, kind, job_id, batch_id, status, error_code)
VALUES (?,?,?,?,?,?,?,?,?)`,
		run.ID, run.ScheduleID,
		run.Due.UTC().Format(time.RFC3339), run.Started.UTC().Format(time.RFC3339),
		nullStr(run.Kind), nullStr(run.JobID), nullStr(run.BatchID), run.Status, nullStr(run.ErrorCode))
	return err
}

func (s *Store) InsertScheduleRunTx(ctx context.Context, tx *sql.Tx, run *ScheduleRun) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO schedule_runs
(id, schedule_id, due, started, kind, job_id, batch_id, status, error_code)
VALUES (?,?,?,?,?,?,?,?,?)`,
		run.ID, run.ScheduleID,
		run.Due.UTC().Format(time.RFC3339), run.Started.UTC().Format(time.RFC3339),
		nullStr(run.Kind), nullStr(run.JobID), nullStr(run.BatchID), run.Status, nullStr(run.ErrorCode))
	return err
}

func (s *Store) UpdateScheduleRun(ctx context.Context, run *ScheduleRun) error {
	_, err := s.db.ExecContext(ctx, `UPDATE schedule_runs SET kind=?, job_id=?, batch_id=?, status=?, error_code=? WHERE id=?`,
		nullStr(run.Kind), nullStr(run.JobID), nullStr(run.BatchID), run.Status, nullStr(run.ErrorCode), run.ID)
	return err
}

func (s *Store) UpdateScheduleLastDueTx(ctx context.Context, tx *sql.Tx, id string, lastDue time.Time, updatedAt time.Time) error {
	ld := lastDue.UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx, `UPDATE schedules SET last_due=?, updated_at=? WHERE id=?`,
		ld, updatedAt.UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) ListScheduleRuns(ctx context.Context, scheduleID string, limit int) ([]*ScheduleRun, error) {
	q := `SELECT id, schedule_id, due, started, kind, job_id, batch_id, status, error_code
FROM schedule_runs WHERE schedule_id=? ORDER BY due DESC`
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduleRuns(rows)
}

// ScheduleNameByJobID returns the schedule name for a single-job run, or empty.
func (s *Store) ScheduleNameByJobID(ctx context.Context, jobID string) (string, error) {
	if jobID == "" {
		return "", nil
	}
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT s.name FROM schedule_runs r
JOIN schedules s ON s.id = r.schedule_id
WHERE r.job_id=? ORDER BY r.started DESC LIMIT 1`, jobID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}

func (s *Store) LastScheduleRun(ctx context.Context, scheduleID string) (*ScheduleRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, schedule_id, due, started, kind, job_id, batch_id, status, error_code
FROM schedule_runs WHERE schedule_id=? ORDER BY due DESC LIMIT 1`, scheduleID)
	var r ScheduleRun
	var due, started string
	var kind, jobID, batchID, ec sql.NullString
	if err := row.Scan(&r.ID, &r.ScheduleID, &due, &started, &kind, &jobID, &batchID, &r.Status, &ec); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.Due, _ = time.Parse(time.RFC3339, due)
	r.Started, _ = time.Parse(time.RFC3339, started)
	r.Kind, r.JobID, r.BatchID, r.ErrorCode = kind.String, jobID.String, batchID.String, ec.String
	return &r, nil
}

func scanScheduleRuns(rows *sql.Rows) ([]*ScheduleRun, error) {
	var out []*ScheduleRun
	for rows.Next() {
		var r ScheduleRun
		var due, started string
		var kind, jobID, batchID, ec sql.NullString
		if err := rows.Scan(&r.ID, &r.ScheduleID, &due, &started, &kind, &jobID, &batchID, &r.Status, &ec); err != nil {
			return nil, err
		}
		r.Due, _ = time.Parse(time.RFC3339, due)
		r.Started, _ = time.Parse(time.RFC3339, started)
		r.Kind, r.JobID, r.BatchID, r.ErrorCode = kind.String, jobID.String, batchID.String, ec.String
		out = append(out, &r)
	}
	return out, nil
}

// PurgeOldScheduleRuns deletes run records older than before (schedules kept).
func (s *Store) PurgeOldScheduleRuns(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedule_runs WHERE started < ?`,
		before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
