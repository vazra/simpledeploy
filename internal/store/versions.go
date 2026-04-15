package store

import (
	"database/sql"
	"fmt"
	"time"
)

// ComposeVersion holds a stored snapshot of a compose file.
type ComposeVersion struct {
	ID        int64     `json:"id"`
	AppID     int64     `json:"app_id"`
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

// DeployEvent records a deploy/rollback action.
type DeployEvent struct {
	ID        int64     `json:"id"`
	AppSlug   string    `json:"app_slug"`
	Action    string    `json:"action"`
	UserID    *int64    `json:"user_id"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateComposeVersion inserts a new version for the app, auto-incrementing the
// version number per app, then prunes versions beyond the most recent 10.
func (s *Store) CreateComposeVersion(appID int64, content, hash string) error {
	_, err := s.db.Exec(`
		INSERT INTO compose_versions (app_id, version, content, hash)
		VALUES (
			?,
			COALESCE((SELECT MAX(version) FROM compose_versions WHERE app_id = ?), 0) + 1,
			?,
			?
		)
	`, appID, appID, content, hash)
	if err != nil {
		return fmt.Errorf("insert compose version: %w", err)
	}

	// prune to 10 most recent
	_, err = s.db.Exec(`
		DELETE FROM compose_versions
		WHERE app_id = ?
		  AND id NOT IN (
			SELECT id FROM compose_versions
			WHERE app_id = ?
			ORDER BY version DESC
			LIMIT 10
		  )
	`, appID, appID)
	if err != nil {
		return fmt.Errorf("prune compose versions: %w", err)
	}

	return nil
}

// ListComposeVersions returns versions for an app ordered newest first.
func (s *Store) ListComposeVersions(appID int64) ([]ComposeVersion, error) {
	rows, err := s.db.Query(`
		SELECT id, app_id, version, content, hash, created_at
		FROM compose_versions
		WHERE app_id = ?
		ORDER BY version DESC
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("list compose versions: %w", err)
	}
	defer rows.Close()

	var versions []ComposeVersion
	for rows.Next() {
		var v ComposeVersion
		if err := rows.Scan(&v.ID, &v.AppID, &v.Version, &v.Content, &v.Hash, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan compose version: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// GetComposeVersion returns a single version by its primary key.
func (s *Store) GetComposeVersion(id int64) (*ComposeVersion, error) {
	var v ComposeVersion
	err := s.db.QueryRow(`
		SELECT id, app_id, version, content, hash, created_at
		FROM compose_versions WHERE id = ?
	`, id).Scan(&v.ID, &v.AppID, &v.Version, &v.Content, &v.Hash, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("compose version %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get compose version: %w", err)
	}
	return &v, nil
}

// DeleteComposeVersion removes a single version by ID.
func (s *Store) DeleteComposeVersion(id int64) error {
	res, err := s.db.Exec(`DELETE FROM compose_versions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete compose version: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("compose version %d not found", id)
	}
	return nil
}

// CreateDeployEvent records an action (deploy/rollback) for an app.
func (s *Store) CreateDeployEvent(appSlug, action string, userID *int64, detail string) error {
	_, err := s.db.Exec(`
		INSERT INTO deploy_events (app_slug, action, user_id, detail)
		VALUES (?, ?, ?, ?)
	`, appSlug, action, userID, detail)
	if err != nil {
		return fmt.Errorf("insert deploy event: %w", err)
	}
	return nil
}

// ListDeployEvents returns the 50 most recent events for an app.
func (s *Store) ListDeployEvents(appSlug string) ([]DeployEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, app_slug, action, user_id, detail, created_at
		FROM deploy_events
		WHERE app_slug = ?
		ORDER BY id DESC
		LIMIT 50
	`, appSlug)
	if err != nil {
		return nil, fmt.Errorf("list deploy events: %w", err)
	}
	defer rows.Close()

	var events []DeployEvent
	for rows.Next() {
		var e DeployEvent
		var userID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.AppSlug, &e.Action, &userID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan deploy event: %w", err)
		}
		if userID.Valid {
			e.UserID = &userID.Int64
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
