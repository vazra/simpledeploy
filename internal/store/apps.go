package store

import (
	"database/sql"
	"fmt"
	"time"
)

type App struct {
	ID          int64
	Name        string
	Slug        string
	ComposePath string
	Status      string
	Domain      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UpsertApp inserts or updates an app by slug and replaces its labels atomically.
func (s *Store) UpsertApp(app *App, labels map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRow(`
		INSERT INTO apps (name, slug, compose_path, status, domain, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(slug) DO UPDATE SET
			name         = excluded.name,
			compose_path = excluded.compose_path,
			status       = excluded.status,
			domain       = excluded.domain,
			updated_at   = excluded.updated_at
		RETURNING id
	`, app.Name, app.Slug, app.ComposePath, app.Status, nullString(app.Domain)).Scan(&id)
	if err != nil {
		return fmt.Errorf("upsert app: %w", err)
	}
	app.ID = id

	if _, err := tx.Exec(`DELETE FROM app_labels WHERE app_id = ?`, id); err != nil {
		return fmt.Errorf("delete labels: %w", err)
	}

	for k, v := range labels {
		if _, err := tx.Exec(
			`INSERT INTO app_labels (app_id, key, value) VALUES (?, ?, ?)`,
			id, k, v,
		); err != nil {
			return fmt.Errorf("insert label %q: %w", k, err)
		}
	}

	return tx.Commit()
}

// GetAppByID returns the app with the given id or an error if not found.
func (s *Store) GetAppByID(id int64) (*App, error) {
	var a App
	var domain sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, slug, compose_path, status, domain, created_at, updated_at
		FROM apps WHERE id = ?
	`, id).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ComposePath, &a.Status,
		&domain, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("app %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get app by id: %w", err)
	}
	if domain.Valid {
		a.Domain = domain.String
	}
	return &a, nil
}

// GetAppBySlug returns the app with the given slug or an error if not found.
func (s *Store) GetAppBySlug(slug string) (*App, error) {
	var a App
	var domain sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, slug, compose_path, status, domain, created_at, updated_at
		FROM apps WHERE slug = ?
	`, slug).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ComposePath, &a.Status,
		&domain, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("app %q not found", slug)
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	if domain.Valid {
		a.Domain = domain.String
	}
	return &a, nil
}

// ListApps returns all apps ordered by name.
func (s *Store) ListApps() ([]App, error) {
	rows, err := s.db.Query(`
		SELECT id, name, slug, compose_path, status, domain, created_at, updated_at
		FROM apps ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var a App
		var domain sql.NullString
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Slug, &a.ComposePath, &a.Status,
			&domain, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		if domain.Valid {
			a.Domain = domain.String
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// DeleteApp deletes the app with the given slug. Returns an error if not found.
func (s *Store) DeleteApp(slug string) error {
	res, err := s.db.Exec(`DELETE FROM apps WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("app %q not found", slug)
	}
	return nil
}

// UpdateAppStatus updates the status field of the app with the given slug.
func (s *Store) UpdateAppStatus(slug, status string) error {
	res, err := s.db.Exec(
		`UPDATE apps SET status = ?, updated_at = datetime('now') WHERE slug = ?`,
		status, slug,
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("app %q not found", slug)
	}
	return nil
}

// GetAppLabels returns all labels for the app with the given slug.
func (s *Store) GetAppLabels(slug string) (map[string]string, error) {
	rows, err := s.db.Query(`
		SELECT al.key, al.value
		FROM app_labels al
		JOIN apps a ON a.id = al.app_id
		WHERE a.slug = ?
	`, slug)
	if err != nil {
		return nil, fmt.Errorf("get labels: %w", err)
	}
	defer rows.Close()

	labels := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels[k] = v
	}
	return labels, rows.Err()
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
