package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Template is a saved prompt with blanks (V7.3).
type Template struct {
	Name         string
	Body         string
	DefaultsJSON string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s *Store) InsertTemplate(ctx context.Context, t *Template) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO templates (name, body, defaults_json, created_at, updated_at)
VALUES (?,?,?,?,?)`,
		t.Name, t.Body, t.DefaultsJSON,
		t.CreatedAt.UTC().Format(time.RFC3339), t.UpdatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetTemplate(ctx context.Context, name string) (*Template, error) {
	row := s.db.QueryRowContext(ctx, `SELECT name, body, defaults_json, created_at, updated_at FROM templates WHERE name=?`, name)
	var t Template
	var ca, ua string
	var def sql.NullString
	if err := row.Scan(&t.Name, &t.Body, &def, &ca, &ua); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, ca)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
	t.DefaultsJSON = def.String
	return &t, nil
}

func (s *Store) ListTemplates(ctx context.Context) ([]*Template, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, body, defaults_json, created_at, updated_at FROM templates ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Template
	for rows.Next() {
		var t Template
		var ca, ua string
		var def sql.NullString
		if err := rows.Scan(&t.Name, &t.Body, &def, &ca, &ua); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, ua)
		t.DefaultsJSON = def.String
		out = append(out, &t)
	}
	return out, nil
}

func (s *Store) UpdateTemplate(ctx context.Context, t *Template) error {
	res, err := s.db.ExecContext(ctx, `UPDATE templates SET body=?, defaults_json=?, updated_at=? WHERE name=?`,
		t.Body, t.DefaultsJSON, t.UpdatedAt.UTC().Format(time.RFC3339), t.Name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTemplate(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM templates WHERE name=?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountTemplates(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM templates`).Scan(&n)
	return n, err
}