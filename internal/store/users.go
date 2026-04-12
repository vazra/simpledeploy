package store

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	DisplayName  string
	Email        string
	CreatedAt    time.Time
}

type APIKeyRecord struct {
	ID        int64
	UserID    int64
	KeyHash   string
	Name      string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// CreateUser inserts a new user and returns it.
func (s *Store) CreateUser(username, passwordHash, role, displayName, email string) (*User, error) {
	var u User
	err := s.db.QueryRow(`
		INSERT INTO users (username, password_hash, role, display_name, email)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id, username, password_hash, role, display_name, email, created_at
	`, username, passwordHash, role, displayName, email).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// GetUserByUsername returns the user with the given username.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	var u User
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, role, display_name, email, created_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}

// GetUserByID returns the user with the given ID.
func (s *Store) GetUserByID(id int64) (*User, error) {
	var u User
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, role, display_name, email, created_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// ListUsers returns all users ordered by username, excluding password_hash.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, username, role, display_name, email, created_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// DeleteUser deletes the user with the given ID.
func (s *Store) DeleteUser(id int64) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

// UserCount returns the total number of users.
func (s *Store) UserCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("user count: %w", err)
	}
	return count, nil
}

// CreateAPIKey inserts a new API key record and returns it.
func (s *Store) CreateAPIKey(userID int64, keyHash, name string) (*APIKeyRecord, error) {
	var k APIKeyRecord
	var expiresAt sql.NullTime
	err := s.db.QueryRow(`
		INSERT INTO api_keys (user_id, key_hash, name)
		VALUES (?, ?, ?)
		RETURNING id, user_id, key_hash, name, created_at, expires_at
	`, userID, keyHash, name).Scan(
		&k.ID, &k.UserID, &k.KeyHash, &k.Name, &k.CreatedAt, &expiresAt,
	)
	if expiresAt.Valid {
		t := expiresAt.Time
		k.ExpiresAt = &t
	}
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &k, nil
}

// GetAPIKeyByHash returns the key record and associated user for the given hash.
func (s *Store) GetAPIKeyByHash(hash string) (*APIKeyRecord, *User, error) {
	var k APIKeyRecord
	var u User
	var expiresAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT
			ak.id, ak.user_id, ak.key_hash, ak.name, ak.created_at, ak.expires_at,
			u.id, u.username, u.password_hash, u.role, u.display_name, u.email, u.created_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.key_hash = ?
	`, hash).Scan(
		&k.ID, &k.UserID, &k.KeyHash, &k.Name, &k.CreatedAt, &expiresAt,
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("api key not found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get api key by hash: %w", err)
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		k.ExpiresAt = &t
	}
	return &k, &u, nil
}

// ListAPIKeysByUser returns all API keys for the given user.
func (s *Store) ListAPIKeysByUser(userID int64) ([]APIKeyRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, key_hash, name, created_at, expires_at
		FROM api_keys WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKeyRecord
	for rows.Next() {
		var k APIKeyRecord
		var expiresAt sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyHash, &k.Name, &k.CreatedAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			k.ExpiresAt = &t
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DeleteAPIKey deletes the API key with the given ID, scoped to the owning user.
// super_admin can delete any key by passing userID=0.
func (s *Store) DeleteAPIKey(id, userID int64) error {
	var query string
	var args []any
	if userID == 0 {
		query = `DELETE FROM api_keys WHERE id = ?`
		args = []any{id}
	} else {
		query = `DELETE FROM api_keys WHERE id = ? AND user_id = ?`
		args = []any{id, userID}
	}
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("api key %d not found", id)
	}
	return nil
}

// GrantAppAccess grants a user access to an app.
func (s *Store) GrantAppAccess(userID, appID int64) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO user_app_access (user_id, app_id) VALUES (?, ?)
	`, userID, appID)
	if err != nil {
		return fmt.Errorf("grant app access: %w", err)
	}
	return nil
}

// RevokeAppAccess revokes a user's access to an app.
func (s *Store) RevokeAppAccess(userID, appID int64) error {
	_, err := s.db.Exec(`
		DELETE FROM user_app_access WHERE user_id = ? AND app_id = ?
	`, userID, appID)
	if err != nil {
		return fmt.Errorf("revoke app access: %w", err)
	}
	return nil
}

// HasAppAccess returns true if the user has access to the app (by slug).
// super_admins always return true.
func (s *Store) HasAppAccess(userID int64, appSlug string) (bool, error) {
	var role string
	err := s.db.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get user role: %w", err)
	}
	if role == "super_admin" {
		return true, nil
	}

	var count int
	err = s.db.QueryRow(`
		SELECT COUNT(*)
		FROM user_app_access uaa
		JOIN apps a ON a.id = uaa.app_id
		WHERE uaa.user_id = ? AND a.slug = ?
	`, userID, appSlug).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check app access: %w", err)
	}
	return count > 0, nil
}

// EmailTaken returns true if the email is already used by another user.
func (s *Store) EmailTaken(email string, excludeID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ? AND id != ?`, email, excludeID).Scan(&count)
	return count > 0, err
}

// UpdateProfile updates the user's display name and email.
func (s *Store) UpdateProfile(id int64, displayName, email string) error {
	res, err := s.db.Exec(`UPDATE users SET display_name = ?, email = ? WHERE id = ?`, displayName, email, id)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

// UpdateUserRole updates the user's role.
func (s *Store) UpdateUserRole(id int64, role string) error {
	res, err := s.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

// UpdatePassword updates the user's password hash.
func (s *Store) UpdatePassword(id int64, newHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, newHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}

// GetUserAppSlugs returns the list of app slugs accessible to the given user.
func (s *Store) GetUserAppSlugs(userID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT a.slug
		FROM apps a
		JOIN user_app_access uaa ON uaa.app_id = a.id
		WHERE uaa.user_id = ?
		ORDER BY a.slug
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get user app slugs: %w", err)
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("scan slug: %w", err)
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}

