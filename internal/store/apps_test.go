package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndGetApp(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "my-app",
		Slug:        "my-app",
		ComposePath: "/apps/my-app/docker-compose.yml",
		Status:      "stopped",
		Domain:      "my-app.example.com",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	if app.ID == 0 {
		t.Fatal("expected app.ID to be set after upsert")
	}

	got, err := s.GetAppBySlug("my-app")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	if got.ID != app.ID {
		t.Errorf("ID = %d, want %d", got.ID, app.ID)
	}
	if got.Name != app.Name {
		t.Errorf("Name = %q, want %q", got.Name, app.Name)
	}
	if got.ComposePath != app.ComposePath {
		t.Errorf("ComposePath = %q, want %q", got.ComposePath, app.ComposePath)
	}
	if got.Status != app.Status {
		t.Errorf("Status = %q, want %q", got.Status, app.Status)
	}
	if got.Domain != app.Domain {
		t.Errorf("Domain = %q, want %q", got.Domain, app.Domain)
	}
}

func TestUpsertAppUpdate(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "app-one",
		Slug:        "app-one",
		ComposePath: "/apps/app-one/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp (insert): %v", err)
	}
	firstID := app.ID

	app.Name = "app-one-renamed"
	app.ComposePath = "/new/path/docker-compose.yml"
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp (update): %v", err)
	}
	if app.ID != firstID {
		t.Errorf("ID changed: got %d, want %d", app.ID, firstID)
	}

	got, err := s.GetAppBySlug("app-one")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	if got.Name != "app-one-renamed" {
		t.Errorf("Name = %q, want app-one-renamed", got.Name)
	}
	if got.ComposePath != "/new/path/docker-compose.yml" {
		t.Errorf("ComposePath = %q, want /new/path/docker-compose.yml", got.ComposePath)
	}
}

func TestListApps(t *testing.T) {
	s := newTestStore(t)

	for _, slug := range []string{"beta-app", "alpha-app"} {
		if err := s.UpsertApp(&App{
			Name:        slug,
			Slug:        slug,
			ComposePath: "/apps/" + slug + "/docker-compose.yml",
			Status:      "stopped",
		}, nil); err != nil {
			t.Fatalf("UpsertApp %q: %v", slug, err)
		}
	}

	apps, err := s.ListApps()
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("len(apps) = %d, want 2", len(apps))
	}
	if apps[0].Slug != "alpha-app" {
		t.Errorf("apps[0].Slug = %q, want alpha-app (ordered by name)", apps[0].Slug)
	}
}

func TestDeleteApp(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "doomed",
		Slug:        "doomed",
		ComposePath: "/apps/doomed/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	if err := s.DeleteApp("doomed"); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	if _, err := s.GetAppBySlug("doomed"); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestGetAppBySlugNotFound(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.GetAppBySlug("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent slug, got nil")
	}
}

func TestGetAppLabels(t *testing.T) {
	s := newTestStore(t)

	labels := map[string]string{
		"env":     "production",
		"team":    "platform",
		"version": "1.2.3",
	}
	app := &App{
		Name:        "labeled",
		Slug:        "labeled",
		ComposePath: "/apps/labeled/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, labels); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	got, err := s.GetAppLabels("labeled")
	if err != nil {
		t.Fatalf("GetAppLabels: %v", err)
	}
	if len(got) != len(labels) {
		t.Fatalf("len(labels) = %d, want %d", len(got), len(labels))
	}
	for k, v := range labels {
		if got[k] != v {
			t.Errorf("label[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestApp_ArchivedAtRoundtrip(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "archivable",
		Slug:        "archivable",
		ComposePath: "/apps/archivable/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	got, err := s.GetAppBySlug("archivable")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	if got.ArchivedAt.Valid {
		t.Fatalf("expected ArchivedAt invalid on fresh insert, got %v", got.ArchivedAt.Time)
	}

	if _, err := s.db.Exec(`UPDATE apps SET archived_at = ? WHERE slug = ?`, time.Now().UTC(), "archivable"); err != nil {
		t.Fatalf("set archived_at: %v", err)
	}

	got, err = s.GetAppBySlug("archivable")
	if err != nil {
		t.Fatalf("GetAppBySlug after archive: %v", err)
	}
	if !got.ArchivedAt.Valid {
		t.Fatalf("expected ArchivedAt valid after update")
	}

	apps, err := s.ListAppsWithOptions(ListAppsOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAppsWithOptions: %v", err)
	}
	if len(apps) != 1 || !apps[0].ArchivedAt.Valid {
		t.Fatalf("ListAppsWithOptions did not surface ArchivedAt: %+v", apps)
	}
}

func TestListApps_ExcludesArchivedByDefault(t *testing.T) {
	s := newTestStore(t)

	for _, slug := range []string{"keep", "archived"} {
		if err := s.UpsertApp(&App{
			Name:        slug,
			Slug:        slug,
			ComposePath: "/apps/" + slug + "/docker-compose.yml",
			Status:      "stopped",
		}, nil); err != nil {
			t.Fatalf("UpsertApp %q: %v", slug, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE apps SET archived_at = ? WHERE slug = ?`, time.Now().UTC(), "archived"); err != nil {
		t.Fatalf("set archived_at: %v", err)
	}

	apps, err := s.ListApps()
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("len(apps) = %d, want 1", len(apps))
	}
	if apps[0].Slug != "keep" {
		t.Errorf("apps[0].Slug = %q, want keep", apps[0].Slug)
	}
}

func TestListAppsWithOptions_IncludeArchived(t *testing.T) {
	s := newTestStore(t)

	for _, slug := range []string{"keep", "archived"} {
		if err := s.UpsertApp(&App{
			Name:        slug,
			Slug:        slug,
			ComposePath: "/apps/" + slug + "/docker-compose.yml",
			Status:      "stopped",
		}, nil); err != nil {
			t.Fatalf("UpsertApp %q: %v", slug, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE apps SET archived_at = ? WHERE slug = ?`, time.Now().UTC(), "archived"); err != nil {
		t.Fatalf("set archived_at: %v", err)
	}

	apps, err := s.ListAppsWithOptions(ListAppsOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAppsWithOptions: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("len(apps) = %d, want 2", len(apps))
	}
}

func TestListArchivedApps(t *testing.T) {
	s := newTestStore(t)

	for _, slug := range []string{"keep", "archived"} {
		if err := s.UpsertApp(&App{
			Name:        slug,
			Slug:        slug,
			ComposePath: "/apps/" + slug + "/docker-compose.yml",
			Status:      "stopped",
		}, nil); err != nil {
			t.Fatalf("UpsertApp %q: %v", slug, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE apps SET archived_at = ? WHERE slug = ?`, time.Now().UTC(), "archived"); err != nil {
		t.Fatalf("set archived_at: %v", err)
	}

	apps, err := s.ListArchivedApps()
	if err != nil {
		t.Fatalf("ListArchivedApps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("len(apps) = %d, want 1", len(apps))
	}
	if apps[0].Slug != "archived" {
		t.Errorf("apps[0].Slug = %q, want archived", apps[0].Slug)
	}
}

func TestMarkAppArchived(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "to-archive",
		Slug:        "to-archive",
		ComposePath: "/apps/to-archive/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	now := time.Now().UTC()
	if err := s.MarkAppArchived("to-archive", now); err != nil {
		t.Fatalf("MarkAppArchived: %v", err)
	}

	got, err := s.GetAppBySlug("to-archive")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	if !got.ArchivedAt.Valid {
		t.Fatal("expected ArchivedAt valid after MarkAppArchived")
	}
	if delta := got.ArchivedAt.Time.Sub(now); delta < -2*time.Second || delta > 2*time.Second {
		t.Errorf("ArchivedAt = %v, want near %v", got.ArchivedAt.Time, now)
	}
}

func TestPurgeApp_CascadesHistory(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "purgeable",
		Slug:        "purgeable",
		ComposePath: "/apps/purgeable/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, map[string]string{"env": "prod"}); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	// user + access
	res, err := s.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('u1','h','admin')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO user_app_access (user_id, app_id) VALUES (?, ?)`, uid, app.ID); err != nil {
		t.Fatalf("insert user_app_access: %v", err)
	}

	// deploy_events
	if _, err := s.db.Exec(`INSERT INTO deploy_events (app_slug, action) VALUES (?, 'deploy')`, app.Slug); err != nil {
		t.Fatalf("insert deploy_events: %v", err)
	}

	// audit_log
	if _, err := s.db.Exec(`INSERT INTO audit_log (app_id, app_slug, actor_source, category, action, summary) VALUES (?, ?, 'user', 'app', 'deploy', 'x')`, app.ID, app.Slug); err != nil {
		t.Fatalf("insert audit_log: %v", err)
	}

	// compose_versions
	cvRes, err := s.db.Exec(`INSERT INTO compose_versions (app_id, version, content, hash) VALUES (?, 1, 'c', 'h')`, app.ID)
	if err != nil {
		t.Fatalf("insert compose_versions: %v", err)
	}
	_ = cvRes

	// backup_configs + backup_runs
	bcRes, err := s.db.Exec(`INSERT INTO backup_configs (app_id, strategy, target, schedule_cron) VALUES (?, 'volume', 'local', '0 0 * * *')`, app.ID)
	if err != nil {
		t.Fatalf("insert backup_configs: %v", err)
	}
	bcID, _ := bcRes.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO backup_runs (backup_config_id, status) VALUES (?, 'success')`, bcID); err != nil {
		t.Fatalf("insert backup_runs: %v", err)
	}

	// alert_rules + alert_history (alert_rules requires webhook_id)
	whRes, err := s.db.Exec(`INSERT INTO webhooks (name, type, url) VALUES ('w', 'slack', 'http://x')`)
	if err != nil {
		t.Fatalf("insert webhook: %v", err)
	}
	whID, _ := whRes.LastInsertId()
	arRes, err := s.db.Exec(`INSERT INTO alert_rules (app_id, metric, operator, threshold, duration_sec, webhook_id) VALUES (?, 'cpu', '>', 50, 60, ?)`, app.ID, whID)
	if err != nil {
		t.Fatalf("insert alert_rules: %v", err)
	}
	arID, _ := arRes.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO alert_history (rule_id, value, app_slug) VALUES (?, 99.0, ?)`, arID, app.Slug); err != nil {
		t.Fatalf("insert alert_history: %v", err)
	}

	if err := s.PurgeApp(app.Slug); err != nil {
		t.Fatalf("PurgeApp: %v", err)
	}

	checks := []struct {
		name  string
		query string
		args  []any
	}{
		{"apps", `SELECT COUNT(*) FROM apps WHERE slug = ?`, []any{app.Slug}},
		{"app_labels", `SELECT COUNT(*) FROM app_labels WHERE app_id = ?`, []any{app.ID}},
		{"user_app_access", `SELECT COUNT(*) FROM user_app_access WHERE app_id = ?`, []any{app.ID}},
		{"deploy_events", `SELECT COUNT(*) FROM deploy_events WHERE app_slug = ?`, []any{app.Slug}},
		{"audit_log", `SELECT COUNT(*) FROM audit_log WHERE app_slug = ? OR app_id = ?`, []any{app.Slug, app.ID}},
		{"compose_versions", `SELECT COUNT(*) FROM compose_versions WHERE app_id = ?`, []any{app.ID}},
		{"backup_configs", `SELECT COUNT(*) FROM backup_configs WHERE app_id = ?`, []any{app.ID}},
		{"backup_runs", `SELECT COUNT(*) FROM backup_runs WHERE backup_config_id = ?`, []any{bcID}},
		{"alert_rules", `SELECT COUNT(*) FROM alert_rules WHERE app_id = ?`, []any{app.ID}},
		{"alert_history", `SELECT COUNT(*) FROM alert_history WHERE app_slug = ? OR rule_id = ?`, []any{app.Slug, arID}},
	}
	for _, c := range checks {
		var n int
		if err := s.db.QueryRow(c.query, c.args...).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.name, err)
		}
		if n != 0 {
			t.Errorf("%s: rows remaining = %d, want 0", c.name, n)
		}
	}
}

func TestPurgeApp_NotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.PurgeApp("nonexistent"); err == nil {
		t.Fatal("expected error for missing app")
	}
}

func TestUpdateAppStatus(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "status-app",
		Slug:        "status-app",
		ComposePath: "/apps/status-app/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	if err := s.UpdateAppStatus("status-app", "running"); err != nil {
		t.Fatalf("UpdateAppStatus: %v", err)
	}

	got, err := s.GetAppBySlug("status-app")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
}
