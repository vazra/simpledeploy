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

func TestWipeConfigForRestore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// Seed config tables.
	_, err = s.db.Exec(`INSERT INTO apps (name, slug, compose_path, status) VALUES ('app', 'app', '/apps/app/docker-compose.yml', 'running')`)
	if err != nil {
		t.Fatalf("insert app: %v", err)
	}
	var appID int64
	s.db.QueryRow("SELECT id FROM apps WHERE slug='app'").Scan(&appID)

	_, err = s.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('admin', 'h', 'super_admin')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err = s.db.Exec(`INSERT INTO webhooks (name, type, url) VALUES ('slack', 'slack', 'https://example.com')`)
	if err != nil {
		t.Fatalf("insert webhook: %v", err)
	}
	var webhookID int64
	s.db.QueryRow("SELECT id FROM webhooks WHERE name='slack'").Scan(&webhookID)

	_, err = s.db.Exec(`INSERT INTO alert_rules (app_id, metric, operator, threshold, duration_sec, webhook_id, enabled) VALUES (?, 'cpu', '>', 80, 60, ?, 1)`, appID, webhookID)
	if err != nil {
		t.Fatalf("insert alert rule: %v", err)
	}

	_, err = s.db.Exec(`INSERT INTO registries (id, name, url, username_enc, password_enc) VALUES ('r1', 'ghcr', 'ghcr.io', 'u', 'p')`)
	if err != nil {
		t.Fatalf("insert registry: %v", err)
	}

	_, err = s.db.Exec(`INSERT INTO db_backup_config (key, value) VALUES ('schedule', '0 3 * * *')`)
	if err != nil {
		t.Fatalf("insert db_backup_config: %v", err)
	}

	// Seed a historical deploy_events row.
	_, err = s.db.Exec(`INSERT INTO deploy_events (app_slug, action, detail) VALUES ('app', 'deploy', 'cli')`)
	if err != nil {
		t.Fatalf("insert deploy_event: %v", err)
	}

	// Wipe.
	if err := s.WipeConfigForRestore(); err != nil {
		t.Fatalf("WipeConfigForRestore: %v", err)
	}

	// Config tables should be empty.
	for _, tbl := range []string{"users", "webhooks", "alert_rules", "registries", "db_backup_config"} {
		var n int
		s.db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n)
		if n != 0 {
			t.Errorf("table %s: want 0 rows after wipe, got %d", tbl, n)
		}
	}

	// Historical tables must survive.
	var nevents int
	s.db.QueryRow("SELECT COUNT(*) FROM deploy_events").Scan(&nevents)
	if nevents != 1 {
		t.Errorf("deploy_events: want 1 row (untouched), got %d", nevents)
	}

	// apps table must survive.
	var napps int
	s.db.QueryRow("SELECT COUNT(*) FROM apps").Scan(&napps)
	if napps != 1 {
		t.Errorf("apps: want 1 row (untouched), got %d", napps)
	}
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
	if version != 22 {
		t.Errorf("migration version = %d, want 22", version)
	}
}
