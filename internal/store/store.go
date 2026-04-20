package store

import (
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

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// WAL mode allows concurrent readers with a single writer.
	// Multiple conns let API reads proceed while reconciler writes.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set auto_vacuum: %w", err)
	}
	if _, err := db.Exec("PRAGMA cache_size=2000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set cache size: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
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

func (s *Store) Close() error {
	return s.db.Close()
}

// IncrementalVacuum reclaims free pages from the database file.
func (s *Store) IncrementalVacuum() error {
	_, err := s.db.Exec("PRAGMA incremental_vacuum")
	return err
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
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
		err = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists)
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

		tx, err := s.db.Begin()
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
