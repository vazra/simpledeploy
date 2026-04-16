package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func insertTestApp(t *testing.T, s *Store, slug string) int64 {
	t.Helper()
	app := &App{Name: slug, Slug: slug, ComposePath: "/tmp/docker-compose.yml", Status: "running"}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	return app.ID
}

func TestCreateAndListComposeVersions(t *testing.T) {
	s := openTestStore(t)
	appID := insertTestApp(t, s, "myapp")

	if err := s.CreateComposeVersion(appID, "content-v1", "hash1"); err != nil {
		t.Fatalf("CreateComposeVersion v1: %v", err)
	}
	if err := s.CreateComposeVersion(appID, "content-v2", "hash2"); err != nil {
		t.Fatalf("CreateComposeVersion v2: %v", err)
	}

	versions, err := s.ListComposeVersions(appID)
	if err != nil {
		t.Fatalf("ListComposeVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	// newest first
	if versions[0].Version != 2 {
		t.Errorf("first version = %d, want 2", versions[0].Version)
	}
	if versions[0].Content != "content-v2" {
		t.Errorf("content = %q, want content-v2", versions[0].Content)
	}
	if versions[1].Version != 1 {
		t.Errorf("second version = %d, want 1", versions[1].Version)
	}
}

func TestComposeVersionPruning(t *testing.T) {
	s := openTestStore(t)
	appID := insertTestApp(t, s, "pruneapp")

	for i := 0; i < 12; i++ {
		if err := s.CreateComposeVersion(appID, "content", "hash"); err != nil {
			t.Fatalf("CreateComposeVersion %d: %v", i+1, err)
		}
	}

	versions, err := s.ListComposeVersions(appID)
	if err != nil {
		t.Fatalf("ListComposeVersions: %v", err)
	}
	if len(versions) != 10 {
		t.Errorf("got %d versions after pruning, want 10", len(versions))
	}
	// should be versions 3-12 (newest 10)
	if versions[0].Version != 12 {
		t.Errorf("latest version = %d, want 12", versions[0].Version)
	}
	if versions[9].Version != 3 {
		t.Errorf("oldest version = %d, want 3", versions[9].Version)
	}
}

func TestGetComposeVersion(t *testing.T) {
	s := openTestStore(t)
	appID := insertTestApp(t, s, "getapp")

	if err := s.CreateComposeVersion(appID, "my-content", "abc123"); err != nil {
		t.Fatalf("CreateComposeVersion: %v", err)
	}

	versions, err := s.ListComposeVersions(appID)
	if err != nil {
		t.Fatalf("ListComposeVersions: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("no versions returned")
	}

	got, err := s.GetComposeVersion(versions[0].ID)
	if err != nil {
		t.Fatalf("GetComposeVersion: %v", err)
	}
	if got.Content != "my-content" {
		t.Errorf("content = %q, want my-content", got.Content)
	}
	if got.Hash != "abc123" {
		t.Errorf("hash = %q, want abc123", got.Hash)
	}

	_, err = s.GetComposeVersion(99999)
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestUpdateComposeVersion(t *testing.T) {
	s := openTestStore(t)
	appID := insertTestApp(t, s, "verapp")

	if err := s.CreateComposeVersion(appID, "content-v1", "hash1"); err != nil {
		t.Fatalf("CreateComposeVersion: %v", err)
	}

	versions, err := s.ListComposeVersions(appID)
	if err != nil {
		t.Fatalf("ListComposeVersions: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("no versions returned")
	}
	v := versions[0]

	// name/notes/env_snapshot should be nil initially
	if v.Name != nil {
		t.Errorf("Name = %v, want nil", v.Name)
	}
	if v.Notes != nil {
		t.Errorf("Notes = %v, want nil", v.Notes)
	}
	if v.EnvSnapshot != nil {
		t.Errorf("EnvSnapshot = %v, want nil", v.EnvSnapshot)
	}

	// update
	if err := s.UpdateComposeVersion(v.ID, "initial deploy", "first version", "FOO=bar\nBAZ=qux"); err != nil {
		t.Fatalf("UpdateComposeVersion: %v", err)
	}

	got, err := s.GetComposeVersion(v.ID)
	if err != nil {
		t.Fatalf("GetComposeVersion: %v", err)
	}
	if got.Name == nil || *got.Name != "initial deploy" {
		t.Errorf("Name = %v, want 'initial deploy'", got.Name)
	}
	if got.Notes == nil || *got.Notes != "first version" {
		t.Errorf("Notes = %v, want 'first version'", got.Notes)
	}
	if got.EnvSnapshot == nil || *got.EnvSnapshot != "FOO=bar\nBAZ=qux" {
		t.Errorf("EnvSnapshot = %v, want 'FOO=bar\\nBAZ=qux'", got.EnvSnapshot)
	}

	// not found
	if err := s.UpdateComposeVersion(99999, "x", "y", "z"); err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestCreateAndListDeployEvents(t *testing.T) {
	s := openTestStore(t)

	if err := s.CreateDeployEvent("myapp", "deploy", nil, ""); err != nil {
		t.Fatalf("CreateDeployEvent deploy: %v", err)
	}

	uid := int64(42)
	if err := s.CreateDeployEvent("myapp", "rollback", &uid, "rollback to version 1"); err != nil {
		t.Fatalf("CreateDeployEvent rollback: %v", err)
	}

	events, err := s.ListDeployEvents("myapp")
	if err != nil {
		t.Fatalf("ListDeployEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	// newest first
	if events[0].Action != "rollback" {
		t.Errorf("first action = %q, want rollback", events[0].Action)
	}
	if events[0].UserID == nil || *events[0].UserID != 42 {
		t.Errorf("user_id mismatch")
	}
	if events[0].Detail != "rollback to version 1" {
		t.Errorf("detail = %q, want 'rollback to version 1'", events[0].Detail)
	}
	if events[1].Action != "deploy" {
		t.Errorf("second action = %q, want deploy", events[1].Action)
	}
	if events[1].UserID != nil {
		t.Errorf("expected nil user_id for deploy event")
	}

	// other app returns nothing
	other, err := s.ListDeployEvents("otherapp")
	if err != nil {
		t.Fatalf("ListDeployEvents other: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("got %d events for otherapp, want 0", len(other))
	}
}
