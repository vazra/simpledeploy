package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db    *sql.DB
	hooks hookState
}

// connPragmas holds the connection-scoped settings. SQLite keeps these per
// connection, not in the file, so a plain `PRAGMA x` through *sql.DB would only
// reach whichever pooled connection ran it. modernc runs every _pragma DSN
// parameter on each connection it opens, so all pooled connections (including
// ones recycled by SetConnMaxLifetime) get the same settings.
//
//   - busy_timeout(5000): retry for up to 5s on lock contention.
//   - foreign_keys(1): enforce REFERENCES / ON DELETE CASCADE / SET NULL.
//   - cache_size(2000): page cache of 2000 pages per connection.
const connPragmas = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=cache_size(2000)"

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?"+connPragmas)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// WAL mode allows concurrent readers with a single writer.
	// Multiple conns let API reads proceed while reconciler writes.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := initDatabase(db); err != nil {
		db.Close()
		return nil, err
	}

	// Restrict database file permissions to owner-only
	if err := os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("chmod db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// initDatabase applies the database-scoped settings, which SQLite persists in
// the file header, so running them on one connection is enough.
func initDatabase(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Close()

	// auto_vacuum is fixed once page 1 is written: by the first table, and also
	// by the journal_mode=WAL switch below. So it can only be chosen on a new,
	// empty file, and must be set before WAL. Existing databases keep their
	// mode; converting one needs a full VACUUM (see VacuumDB).
	var pages int64
	if err := conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return fmt.Errorf("read page_count: %w", err)
	}
	if pages == 0 {
		if _, err := conn.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
			return fmt.Errorf("set auto_vacuum: %w", err)
		}
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("set WAL mode: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// IncrementalVacuum reclaims free pages from the database file. It is a no-op
// on databases whose auto_vacuum mode is NONE (created before auto_vacuum was
// applied correctly) until VacuumDB converts them.
func (s *Store) IncrementalVacuum() error {
	_, err := s.db.Exec("PRAGMA incremental_vacuum")
	return err
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() (err error) {
	ctx := context.Background()
	// All migrations run on one dedicated connection so the foreign_keys
	// toggle below applies to every statement they execute.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Close()

	// Table rebuilds (create new, copy, DROP old, rename) are the only way to
	// change a CHECK or column type in SQLite. With foreign keys on, DROP
	// TABLE runs an implicit DELETE that fires ON DELETE CASCADE / SET NULL on
	// every child table, silently wiping rows that reference the rebuilt table
	// (e.g. 025 rebuilding users would delete api_keys and user_app_access).
	// SQLite's documented procedure is to turn foreign keys off for the
	// duration. The pragma is a no-op inside a transaction, so it is set here,
	// and restored before the connection goes back to the pool.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() {
		if _, ferr := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); ferr != nil && err == nil {
			err = fmt.Errorf("re-enable foreign keys: %w", ferr)
		}
	}()

	_, err = conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var filenames []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			filenames = append(filenames, e.Name())
		}
	}
	sort.Strings(filenames)

	for _, name := range filenames {
		version, err := parseVersion(name)
		if err != nil {
			return fmt.Errorf("parse version from %s: %w", name, err)
		}

		var exists int
		err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if exists > 0 {
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(string(data)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

// WipeConfigForRestore truncates config tables (children first) while leaving
// historical/operational tables (metrics, deploy_events, etc.) untouched.
// Intended for DR import --wipe. Foreign keys are enforced, so the deletes
// cascade: alert_history rows go with their alert_rules, backup_runs with
// their backup_configs, and audit_log.actor_user_id is set NULL for deleted
// users. Deleting children first keeps each statement's cascade small.
func (s *Store) WipeConfigForRestore() error {
	tables := []string{
		"user_app_access",
		"api_keys",
		"users",
		"alert_rules",
		"webhooks",
		"backup_configs",
		"registries",
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("WipeConfigForRestore: begin tx: %w", err)
	}
	for _, t := range tables {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			tx.Rollback()
			return fmt.Errorf("WipeConfigForRestore: delete %s: %w", t, err)
		}
	}
	// Wipe db_backup_config key-value rows.
	if _, err := tx.Exec("DELETE FROM db_backup_config"); err != nil {
		tx.Rollback()
		return fmt.Errorf("WipeConfigForRestore: delete db_backup_config: %w", err)
	}
	return tx.Commit()
}

func parseVersion(filename string) (int, error) {
	// expects format: 001_description.sql
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}
	var v int
	if _, err := fmt.Sscanf(parts[0], "%d", &v); err != nil {
		return 0, fmt.Errorf("invalid version in %s: %w", filename, err)
	}
	return v, nil
}
