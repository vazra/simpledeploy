package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigration025_RenamesAdminToManage seeds the schema up through migration
// 024 (when 'admin' was still valid), inserts an 'admin' user, then opens the
// store normally so migration 025 runs and verifies the role flips to 'manage'
// without granting any new app access.
func TestMigration025_RenamesAdminToManage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open raw DB and run only the legacy schema and 024.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := raw.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer' CHECK(role IN ('super_admin', 'admin', 'viewer')),
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			display_name TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE apps (id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT);
		CREATE TABLE user_app_access (user_id INTEGER, app_id INTEGER, PRIMARY KEY (user_id, app_id));
		CREATE TABLE api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			expires_at DATETIME
		);
	`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('legacy', 'h', 'admin')`); err != nil {
		t.Fatalf("seed admin user: %v", err)
	}
	// Mark migrations 1..24 as applied so Open only runs 025.
	for v := 1; v <= 24; v++ {
		if _, err := raw.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, v); err != nil {
			t.Fatalf("insert migration %d: %v", v, err)
		}
	}
	raw.Close()

	// Now open via the store, which applies migration 025.
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var role string
	if err := s.db.QueryRow(`SELECT role FROM users WHERE username='legacy'`).Scan(&role); err != nil {
		t.Fatalf("read role: %v", err)
	}
	if role != "manage" {
		t.Errorf("role after migration = %q, want manage", role)
	}

	// No automatic app grants should be present.
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM user_app_access`).Scan(&n); err != nil {
		t.Fatalf("count access: %v", err)
	}
	if n != 0 {
		t.Errorf("user_app_access rows = %d, want 0", n)
	}

	// CHECK constraint should now reject 'admin'.
	if _, err := s.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('x','h','admin')`); err == nil {
		t.Error("expected CHECK constraint to reject role='admin' after migration")
	}
}
