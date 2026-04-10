package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Registry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	UsernameEnc string    `json:"-"`
	PasswordEnc string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (s *Store) CreateRegistry(name, url, usernameEnc, passwordEnc string) (*Registry, error) {
	id := newID()
	_, err := s.db.Exec(`
		INSERT INTO registries (id, name, url, username_enc, password_enc)
		VALUES (?, ?, ?, ?, ?)`,
		id, name, url, usernameEnc, passwordEnc,
	)
	if err != nil {
		return nil, fmt.Errorf("insert registry: %w", err)
	}
	return s.GetRegistry(id)
}

func (s *Store) ListRegistries() ([]Registry, error) {
	rows, err := s.db.Query(`SELECT id, name, url, username_enc, password_enc, created_at, updated_at FROM registries ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query registries: %w", err)
	}
	defer rows.Close()

	var regs []Registry
	for rows.Next() {
		var r Registry
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.UsernameEnc, &r.PasswordEnc, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan registry: %w", err)
		}
		regs = append(regs, r)
	}
	return regs, rows.Err()
}

func (s *Store) GetRegistry(id string) (*Registry, error) {
	var r Registry
	err := s.db.QueryRow(`SELECT id, name, url, username_enc, password_enc, created_at, updated_at FROM registries WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.URL, &r.UsernameEnc, &r.PasswordEnc, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get registry: %w", err)
	}
	return &r, nil
}

func (s *Store) GetRegistryByName(name string) (*Registry, error) {
	var r Registry
	err := s.db.QueryRow(`SELECT id, name, url, username_enc, password_enc, created_at, updated_at FROM registries WHERE name = ?`, name).
		Scan(&r.ID, &r.Name, &r.URL, &r.UsernameEnc, &r.PasswordEnc, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get registry by name: %w", err)
	}
	return &r, nil
}

func (s *Store) UpdateRegistry(id, name, url, usernameEnc, passwordEnc string) error {
	res, err := s.db.Exec(`
		UPDATE registries SET name = ?, url = ?, username_enc = ?, password_enc = ?, updated_at = datetime('now')
		WHERE id = ?`,
		name, url, usernameEnc, passwordEnc, id,
	)
	if err != nil {
		return fmt.Errorf("update registry: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("registry not found: %s", id)
	}
	return nil
}

func (s *Store) DeleteRegistry(id string) error {
	res, err := s.db.Exec(`DELETE FROM registries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete registry: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("registry not found: %s", id)
	}
	return nil
}
