package store

import (
	"path/filepath"
	"testing"
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
