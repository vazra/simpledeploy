package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	// verify WAL mode
	var journalMode string
	err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	// verify apps table exists by inserting a row
	_, err = s.db.Exec(
		`INSERT INTO apps (name, slug, compose_path, status) VALUES (?, ?, ?, ?)`,
		"test-app", "test-app", "/tmp/test/docker-compose.yml", "stopped",
	)
	if err != nil {
		t.Fatalf("insert into apps: %v", err)
	}

	// verify we can read it back
	var name, slug, status string
	err = s.db.QueryRow("SELECT name, slug, status FROM apps WHERE slug = ?", "test-app").
		Scan(&name, &slug, &status)
	if err != nil {
		t.Fatalf("select from apps: %v", err)
	}
	if name != "test-app" || status != "stopped" {
		t.Errorf("got name=%q status=%q, want test-app stopped", name, status)
	}
}

func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open() error: %v", err)
	}
	s1.Close()

	// opening again should not fail (migrations already applied)
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open() error: %v", err)
	}
	s2.Close()
}

func TestMigrationVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	var version int
	err = s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if version != 3 {
		t.Errorf("migration version = %d, want 3", version)
	}
}
