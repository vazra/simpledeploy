package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
)

// grabConns checks out n connections at once so each one is a distinct
// pooled connection. They are released on test cleanup, or earlier via
// releaseConns (closing twice is harmless).
func grabConns(t *testing.T, s *Store, n int) []*sql.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conns := make([]*sql.Conn, 0, n)
	for i := 0; i < n; i++ {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	t.Cleanup(func() { releaseConns(conns) })
	return conns
}

func releaseConns(conns []*sql.Conn) {
	for _, c := range conns {
		c.Close()
	}
}

func pragmaInt(t *testing.T, c *sql.Conn, name string) int64 {
	t.Helper()
	var v int64
	if err := c.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&v); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return v
}

// assertConnPragmas verifies the connection-scoped pragmas on every pooled
// connection (MaxOpenConns is 4), not just the one that happened to run them.
func assertConnPragmas(t *testing.T, s *Store) {
	t.Helper()
	conns := grabConns(t, s, 4)
	defer releaseConns(conns)
	for i, c := range conns {
		if got := pragmaInt(t, c, "foreign_keys"); got != 1 {
			t.Errorf("conn %d: foreign_keys = %d, want 1", i, got)
		}
		if got := pragmaInt(t, c, "cache_size"); got != 2000 {
			t.Errorf("conn %d: cache_size = %d, want 2000", i, got)
		}
		if got := pragmaInt(t, c, "busy_timeout"); got != 5000 {
			t.Errorf("conn %d: busy_timeout = %d, want 5000", i, got)
		}
	}
}

func TestConnPragmasApplyToEveryPooledConn(t *testing.T) {
	s := newTestStore(t)
	assertConnPragmas(t, s)
}

// TestForeignKeysEnforcedOnEveryConn runs a cascade and a violating insert on
// each pooled connection. Before the DSN fix only the first connection had
// foreign_keys=ON, so these failed on every other connection.
func TestForeignKeysEnforcedOnEveryConn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for i, c := range grabConns(t, s, 4) {
		slug := fmt.Sprintf("fk-app-%d", i)
		var appID int64
		if err := c.QueryRowContext(ctx,
			`INSERT INTO apps (name, slug, compose_path, status) VALUES (?, ?, '/x', 'running') RETURNING id`,
			slug, slug).Scan(&appID); err != nil {
			t.Fatalf("conn %d: insert app: %v", i, err)
		}
		if _, err := c.ExecContext(ctx, `INSERT INTO app_labels (app_id, key, value) VALUES (?, 'k', 'v')`, appID); err != nil {
			t.Fatalf("conn %d: insert label: %v", i, err)
		}
		if _, err := c.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, appID); err != nil {
			t.Fatalf("conn %d: delete app: %v", i, err)
		}
		var n int
		if err := c.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_labels WHERE app_id = ?`, appID).Scan(&n); err != nil {
			t.Fatalf("conn %d: count labels: %v", i, err)
		}
		if n != 0 {
			t.Errorf("conn %d: app_labels rows after app delete = %d, want 0 (ON DELETE CASCADE)", i, n)
		}
		if _, err := c.ExecContext(ctx, `INSERT INTO app_labels (app_id, key, value) VALUES (987654, 'k', 'v')`); err == nil {
			t.Errorf("conn %d: insert referencing missing app succeeded, want FK error", i)
		}
	}
}

func TestAutoVacuumIncrementalOnNewDB(t *testing.T) {
	s := newTestStore(t)
	c := grabConns(t, s, 1)[0]
	if got := pragmaInt(t, c, "auto_vacuum"); got != 2 {
		t.Errorf("auto_vacuum = %d, want 2 (incremental)", got)
	}
	var mode string
	if err := c.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

// TestAutoVacuumExistingDB: Open must not rewrite an existing database to
// change auto_vacuum (that needs a full VACUUM); the explicit VacuumDB does
// the conversion.
func TestAutoVacuumExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Mimic the old Open: WAL first, then auto_vacuum (too late, ignored).
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA auto_vacuum=INCREMENTAL",
		"CREATE TABLE legacy (x INTEGER)",
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if got := pragmaInt(t, grabConns(t, s, 1)[0], "auto_vacuum"); got != 0 {
		t.Fatalf("auto_vacuum after Open = %d, want 0 (existing DB left as is)", got)
	}

	if err := s.VacuumDB(); err != nil {
		t.Fatalf("VacuumDB: %v", err)
	}
	ctx := context.Background()
	c, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := pragmaInt(t, c, "auto_vacuum"); got != 2 {
		t.Errorf("auto_vacuum after VacuumDB = %d, want 2 (incremental)", got)
	}
	if err := s.IncrementalVacuum(); err != nil {
		t.Errorf("IncrementalVacuum: %v", err)
	}
}

// applyMigrationsUpTo builds a database the way an older binary left it:
// schema_migrations plus migrations 1..upTo.
func applyMigrationsUpTo(t *testing.T, path string, upTo int) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		v, err := parseVersion(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if v > upTo {
			break
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			t.Fatalf("migration %s: %v", e.Name(), err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatal(err)
		}
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func fkViolations(t *testing.T, db *sql.DB) int {
	t.Helper()
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestMigrationTableRebuildKeepsChildRows upgrades a database from before
// migration 019 (rebuilds apps) and 025 (rebuilds users). Those migrations DROP
// the parent table; with foreign keys on, the implicit DELETE cascaded into
// every child table and wiped labels, grants, API keys, versions, etc.
func TestMigrationTableRebuildKeepsChildRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	applyMigrationsUpTo(t, path, 18)

	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO apps (id, name, slug, compose_path, status) VALUES (1, 'a', 'a', '/apps/a/docker-compose.yml', 'running')`,
		`INSERT INTO users (id, username, password_hash, role) VALUES (1, 'legacy', 'h', 'admin')`,
		`INSERT INTO app_labels (app_id, key, value) VALUES (1, 'k', 'v')`,
		`INSERT INTO user_app_access (user_id, app_id) VALUES (1, 1)`,
		`INSERT INTO api_keys (user_id, key_hash, name) VALUES (1, 'hash', 'ci')`,
		`INSERT INTO compose_versions (app_id, version, content, hash) VALUES (1, 1, 'services: {}', 'h1')`,
		`INSERT INTO backup_configs (id, app_id, strategy, target, schedule_cron) VALUES (1, 1, 'volume', 'local', '0 3 * * *')`,
		`INSERT INTO backup_runs (backup_config_id, status) VALUES (1, 'success')`,
		`INSERT INTO webhooks (id, name, type, url) VALUES (1, 'w', 'custom', 'https://example.com')`,
		`INSERT INTO alert_rules (id, app_id, metric, operator, threshold, webhook_id) VALUES (1, 1, 'cpu', '>', 80, 1)`,
		`INSERT INTO alert_history (rule_id, value) VALUES (1, 90)`,
		`INSERT INTO metrics (app_id, ts) VALUES (1, 1000)`,
		`INSERT INTO request_metrics (app_id, ts) VALUES (1, 1000)`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open (runs 019+): %v", err)
	}
	defer s.Close()

	for _, table := range []string{
		"apps", "users", "app_labels", "user_app_access", "api_keys",
		"compose_versions", "backup_configs", "backup_runs", "webhooks",
		"alert_rules", "alert_history", "metrics", "request_metrics",
	} {
		if got := countRows(t, s.db, table); got != 1 {
			t.Errorf("%s rows after migrations = %d, want 1", table, got)
		}
	}
	if n := fkViolations(t, s.db); n != 0 {
		t.Errorf("foreign_key_check after migrations: %d violations, want 0", n)
	}
	var role string
	if err := s.db.QueryRow(`SELECT role FROM users WHERE id = 1`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "manage" {
		t.Errorf("role = %q, want manage", role)
	}
	// The migration connection must go back to the pool with FKs on.
	assertConnPragmas(t, s)
}

// TestOpenWithPreexistingOrphans: databases written while foreign keys were
// only enforced on one pooled connection can hold orphan rows. SQLite does not
// re-check existing rows when enforcement is turned on, so Open must still
// succeed, and code paths that copy FK values out of existing rows (metric
// rollups) must not trip over them.
func TestOpenWithPreexistingOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orphans.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	appID := makeApp(t, s, "live")
	s.Close()

	old := time.Now().Add(-2 * time.Hour).Unix()
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO app_labels (app_id, key, value) VALUES (999, 'k', 'v')`, nil},
		{`INSERT INTO api_keys (user_id, key_hash, name) VALUES (999, 'orphan', 'n')`, nil},
		{`INSERT INTO alert_rules (metric, operator, threshold, webhook_id) VALUES ('cpu', '>', 80, 999)`, nil},
		{`INSERT INTO alert_history (id, rule_id, value) VALUES (500, 999, 1)`, nil},
		{`INSERT INTO audit_log (app_id, actor_user_id, actor_source, category, action, summary) VALUES (999, 999, 'ui', 'compose', 'changed', 's')`, nil},
		{`INSERT INTO metrics (app_id, container_id, ts, tier, cpu_pct) VALUES (999, 'gone', ?, 'raw', 50)`, []any{old}},
		{`INSERT INTO metrics (app_id, container_id, ts, tier, cpu_pct) VALUES (?, 'c1', ?, 'raw', 10)`, []any{appID, old}},
		{`INSERT INTO request_metrics (app_id, ts, tier, count) VALUES (999, ?, 'raw', 5)`, []any{old}},
		{`INSERT INTO request_metrics (app_id, ts, tier, count) VALUES (?, ?, 'raw', 3)`, []any{appID, old}},
	} {
		if _, err := raw.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed orphan %q: %v", q.sql, err)
		}
	}
	raw.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatalf("Open with orphans: %v", err)
	}
	defer s.Close()
	assertConnPragmas(t, s)

	// Orphans are left in place, not silently deleted at startup.
	if n := fkViolations(t, s.db); n == 0 {
		t.Fatal("expected pre-existing orphans to still be reported by foreign_key_check")
	}

	cutoff := time.Now().Add(-time.Hour)
	if err := s.AggregateMetrics(metrics.TierRaw, metrics.Tier1m, cutoff); err != nil {
		t.Fatalf("AggregateMetrics with orphan rows: %v", err)
	}
	if err := s.AggregateRequestMetrics(metrics.TierRaw, metrics.Tier1m, cutoff); err != nil {
		t.Fatalf("AggregateRequestMetrics with orphan rows: %v", err)
	}
	for _, table := range []string{"metrics", "request_metrics"} {
		var raws, live, orphan int
		s.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE tier = 'raw'`).Scan(&raws)
		s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE tier = '1m' AND app_id = ?`, appID).Scan(&live)
		s.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE app_id = 999`).Scan(&orphan)
		if raws != 0 || live != 1 || orphan != 0 {
			t.Errorf("%s after rollup: raw=%d live 1m=%d orphan=%d, want 0/1/0", table, raws, live, orphan)
		}
	}

	// Updates that do not touch the FK column still work on orphan rows.
	if err := s.ResolveAlert(500); err != nil {
		t.Errorf("ResolveAlert on orphan history row: %v", err)
	}
	// New writes are checked.
	if _, err := s.db.Exec(`INSERT INTO app_labels (app_id, key, value) VALUES (999, 'k2', 'v')`); err == nil {
		t.Error("insert referencing missing app succeeded, want FK error")
	}
}

func TestInsertMetricsSkipsMissingApp(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "live")
	gone := makeApp(t, s, "gone")
	if err := s.PurgeApp("gone"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	err := s.InsertMetrics([]metrics.MetricPoint{
		{AppID: &appID, ContainerID: "c1", Ts: now, CPUPct: 1},
		{AppID: nil, ContainerID: "", Ts: now, CPUPct: 2},
		{AppID: &gone, ContainerID: "c2", Ts: now, CPUPct: 3},
	})
	if err != nil {
		t.Fatalf("InsertMetrics with a deleted app in the batch: %v", err)
	}
	var live, system, orphan int
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE app_id = ?`, appID).Scan(&live)
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE app_id IS NULL`).Scan(&system)
	s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE app_id = ?`, gone).Scan(&orphan)
	if live != 1 || system != 1 || orphan != 0 {
		t.Errorf("live=%d system=%d orphan=%d, want 1/1/0", live, system, orphan)
	}
}

func TestInsertRequestMetricsSkipsMissingApp(t *testing.T) {
	s := newTestStore(t)
	appID := makeApp(t, s, "live")
	gone := makeApp(t, s, "gone")
	if err := s.PurgeApp("gone"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	err := s.InsertRequestMetrics([]metrics.RequestMetricPoint{
		{AppID: appID, Ts: now, Count: 1},
		{AppID: gone, Ts: now, Count: 2},
	})
	if err != nil {
		t.Fatalf("InsertRequestMetrics with a deleted app in the batch: %v", err)
	}
	var live, orphan int
	s.db.QueryRow(`SELECT COUNT(*) FROM request_metrics WHERE app_id = ?`, appID).Scan(&live)
	s.db.QueryRow(`SELECT COUNT(*) FROM request_metrics WHERE app_id = ?`, gone).Scan(&orphan)
	if live != 1 || orphan != 0 {
		t.Errorf("live=%d orphan=%d, want 1/0", live, orphan)
	}
}

// TestRecordAuditMissingParents: audit rows referencing an app, user or
// compose version that was deleted before the write must still be recorded,
// with the dangling reference set NULL (the columns' ON DELETE SET NULL),
// instead of failing on the foreign key.
func TestRecordAuditMissingParents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	appID := makeApp(t, s, "live")
	missing := int64(987654)

	id, err := s.RecordAudit(ctx, AuditEntry{
		AppID:            &missing,
		AppSlug:          "gone",
		ActorUserID:      &missing,
		ActorName:        "ghost",
		ActorSource:      "api",
		Category:         "deploy",
		Action:           "rollback",
		Summary:          "s",
		ComposeVersionID: &missing,
	})
	if err != nil {
		t.Fatalf("RecordAudit with missing parents: %v", err)
	}
	got, err := s.GetActivity(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.AppID != nil || got.ActorUserID != nil || got.ComposeVersionID != nil {
		t.Errorf("dangling refs not nulled: app=%v actor=%v version=%v", got.AppID, got.ActorUserID, got.ComposeVersionID)
	}
	if got.AppSlug != "gone" || got.ActorName != "ghost" {
		t.Errorf("denormalized fields lost: slug=%q actor=%q", got.AppSlug, got.ActorName)
	}

	// Existing parents are kept as is.
	id, err = s.RecordAudit(ctx, AuditEntry{AppID: &appID, AppSlug: "live", ActorSource: "api", Category: "deploy", Action: "deployed", Summary: "s"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.GetActivity(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.AppID == nil || *got.AppID != appID {
		t.Errorf("app_id = %v, want %d", got.AppID, appID)
	}
}

// Deleting an alert rule keeps its history (rule_id becomes NULL); rows carry
// a snapshot of the rule.
func TestDeleteAlertRuleKeepsHistory(t *testing.T) {
	s := newTestStore(t)
	wh := &Webhook{Name: "h", Type: "custom", URL: "https://example.com/h"}
	if err := s.CreateWebhook(wh); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	rule := &AlertRule{Metric: "cpu_pct", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: wh.ID, Enabled: true}
	if err := s.CreateAlertRule(rule); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	if _, err := s.CreateAlertHistory(rule.ID, 95, rule); err != nil {
		t.Fatalf("CreateAlertHistory: %v", err)
	}
	if err := s.DeleteAlertRule(rule.ID); err != nil {
		t.Fatalf("DeleteAlertRule: %v", err)
	}
	hist, err := s.ListAlertHistory(nil, 10)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) != 1 || hist[0].RuleID != nil || hist[0].Metric != "cpu_pct" {
		t.Fatalf("history = %+v, want one row with NULL rule_id and the snapshot", hist)
	}
}
