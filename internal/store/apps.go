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
	ComposeHash string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  sql.NullTime
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
		INSERT INTO apps (name, slug, compose_path, status, domain, compose_hash, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(slug) DO UPDATE SET
			name         = excluded.name,
			compose_path = excluded.compose_path,
			status       = excluded.status,
			domain       = excluded.domain,
			compose_hash = excluded.compose_hash,
			updated_at   = excluded.updated_at
		RETURNING id
	`, app.Name, app.Slug, app.ComposePath, app.Status, nullString(app.Domain), app.ComposeHash).Scan(&id)
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
		SELECT id, name, slug, compose_path, status, domain, compose_hash, created_at, updated_at, archived_at
		FROM apps WHERE id = ?
	`, id).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ComposePath, &a.Status,
		&domain, &a.ComposeHash, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt,
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
		SELECT id, name, slug, compose_path, status, domain, compose_hash, created_at, updated_at, archived_at
		FROM apps WHERE slug = ?
	`, slug).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ComposePath, &a.Status,
		&domain, &a.ComposeHash, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt,
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

// ListAppsOptions controls archive filtering for list queries.
type ListAppsOptions struct {
	IncludeArchived bool
	OnlyArchived    bool
}

// ListApps returns all non-archived apps ordered by name.
func (s *Store) ListApps() ([]App, error) {
	return s.ListAppsWithOptions(ListAppsOptions{})
}

// ListArchivedApps returns only archived apps ordered by name.
func (s *Store) ListArchivedApps() ([]App, error) {
	return s.ListAppsWithOptions(ListAppsOptions{OnlyArchived: true})
}

// ListAppsWithOptions returns apps filtered by archive state.
func (s *Store) ListAppsWithOptions(opts ListAppsOptions) ([]App, error) {
	where := "WHERE archived_at IS NULL"
	if opts.OnlyArchived {
		where = "WHERE archived_at IS NOT NULL"
	} else if opts.IncludeArchived {
		where = ""
	}
	query := `
		SELECT id, name, slug, compose_path, status, domain, compose_hash, created_at, updated_at, archived_at
		FROM apps ` + where + ` ORDER BY name
	`
	rows, err := s.db.Query(query)
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
			&domain, &a.ComposeHash, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt,
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

// MarkAppArchived sets archived_at on the app with the given slug.
func (s *Store) MarkAppArchived(slug string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE apps SET archived_at = ? WHERE slug = ?`, at.UTC(), slug)
	return err
}

// PurgeApp deletes an app and every related history row (audit, deploy events,
// compose versions, backups, alerts, labels, access). Use when removing an app
// for good. Runs in a single transaction.
func (s *Store) PurgeApp(slug string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var appID int64
	if err := tx.QueryRow(`SELECT id FROM apps WHERE slug = ?`, slug).Scan(&appID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("app %q not found", slug)
		}
		return fmt.Errorf("lookup app id: %w", err)
	}

	type stmt struct {
		query string
		arg   any
	}
	// audit_log is preserved across app purge for forensic continuity.
	// Migration 020 declared `app_id ... ON DELETE SET NULL`; the row is
	// also denormalized with `app_slug` so it remains identifiable after
	// the apps row is gone. Pre-existing app-id refs are nulled below.
	statements := []stmt{
		{`UPDATE audit_log SET app_id = NULL WHERE app_id = ?`, appID},
		{`DELETE FROM deploy_events WHERE app_slug = ?`, slug},
		{`DELETE FROM backup_runs WHERE backup_config_id IN (SELECT id FROM backup_configs WHERE app_id = ?)`, appID},
		{`DELETE FROM compose_versions WHERE app_id = ?`, appID},
		{`DELETE FROM alert_history WHERE app_slug = ? OR rule_id IN (SELECT id FROM alert_rules WHERE app_id = ?)`, nil},
		{`DELETE FROM backup_configs WHERE app_id = ?`, appID},
		{`DELETE FROM alert_rules WHERE app_id = ?`, appID},
		{`DELETE FROM app_labels WHERE app_id = ?`, appID},
		{`DELETE FROM user_app_access WHERE app_id = ?`, appID},
		{`DELETE FROM apps WHERE slug = ?`, slug},
	}
	for _, st := range statements {
		var err error
		switch st.query {
		case `DELETE FROM alert_history WHERE app_slug = ? OR rule_id IN (SELECT id FROM alert_rules WHERE app_id = ?)`:
			_, err = tx.Exec(st.query, slug, appID)
		default:
			_, err = tx.Exec(st.query, st.arg)
		}
		if err != nil {
			return fmt.Errorf("purge %s: %w", st.query, err)
		}
	}
	return tx.Commit()
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

// UpdateAppDisplayName updates the name (display) field of the app with the given slug.
// No-op if the name already matches. Used by configsync ApplyAppSidecar.
func (s *Store) UpdateAppDisplayName(slug, name string) error {
	res, err := s.db.Exec(
		`UPDATE apps SET name = ?, updated_at = datetime('now') WHERE slug = ? AND name <> ?`,
		name, slug, name,
	)
	if err != nil {
		return fmt.Errorf("update app name: %w", err)
	}
	if _, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("rows affected: %w", err)
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
