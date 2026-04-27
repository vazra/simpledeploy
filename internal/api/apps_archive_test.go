package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

func seedArchivedApp(t *testing.T, s *store.Store, slug string) {
	t.Helper()
	if err := s.UpsertApp(&store.App{
		Name:        slug,
		Slug:        slug,
		ComposePath: "/tmp/" + slug + ".yml",
		Status:      "stopped",
	}, nil); err != nil {
		t.Fatalf("UpsertApp %s: %v", slug, err)
	}
	if err := s.MarkAppArchived(slug, time.Now()); err != nil {
		t.Fatalf("MarkAppArchived %s: %v", slug, err)
	}
}

func TestGetApps_ExcludesArchivedByDefault(t *testing.T) {
	srv, s := newTestServer(t)
	if err := s.UpsertApp(&store.App{Name: "live", Slug: "live", ComposePath: "/tmp/l.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert live: %v", err)
	}
	seedArchivedApp(t, s, "gone")

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var apps []map[string]any
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 1 || apps[0]["Slug"] != "live" {
		t.Fatalf("got %+v, want only live", apps)
	}
}

func TestGetApps_IncludeArchivedQuery(t *testing.T) {
	srv, s := newTestServer(t)
	if err := s.UpsertApp(&store.App{Name: "live", Slug: "live", ComposePath: "/tmp/l.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert live: %v", err)
	}
	seedArchivedApp(t, s, "gone")

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps?include_archived=1", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	var apps []map[string]any
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 2 {
		t.Fatalf("got %d, want 2", len(apps))
	}
	var archivedSeen bool
	for _, a := range apps {
		if a["Slug"] == "gone" {
			if a["ArchivedAt"] == nil {
				t.Errorf("archived row missing ArchivedAt: %+v", a)
			}
			archivedSeen = true
		}
	}
	if !archivedSeen {
		t.Fatalf("archived app not in response")
	}
}

func TestGetAppsArchived_ReturnsTombstone(t *testing.T) {
	srv, s := newTestServer(t)
	dataDir := t.TempDir()
	cs := configsync.New(s, t.TempDir(), dataDir)
	t.Cleanup(func() { cs.Close() })
	srv.SetConfigSync(cs)

	seedArchivedApp(t, s, "tombed")
	if err := cs.WriteTombstone("tombed", time.Now()); err != nil {
		t.Fatalf("WriteTombstone: %v", err)
	}
	// Second archived with no tombstone file — handler must tolerate.
	seedArchivedApp(t, s, "lonely")

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/archived", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Fatalf("got %d, want 2", len(resp))
	}
	for _, r := range resp {
		if r["Slug"] == "tombed" {
			if r["tombstone"] == nil {
				t.Errorf("tombed missing tombstone payload")
			}
		}
		if r["Slug"] == "lonely" {
			if r["tombstone"] != nil {
				t.Errorf("lonely should have nil tombstone, got %v", r["tombstone"])
			}
		}
	}
}

func TestPostPurge_409OnNonArchived(t *testing.T) {
	srv, s := newTestServer(t)
	if err := s.UpsertApp(&store.App{Name: "live", Slug: "live", ComposePath: "/tmp/l.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/live/purge", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestPostPurge_204OnArchived(t *testing.T) {
	srv, s := newTestServer(t)
	dataDir := t.TempDir()
	cs := configsync.New(s, t.TempDir(), dataDir)
	t.Cleanup(func() { cs.Close() })
	srv.SetConfigSync(cs)
	srv.SetAudit(audit.NewRecorder(s))

	seedArchivedApp(t, s, "purgeme")
	if err := cs.WriteTombstone("purgeme", time.Now()); err != nil {
		t.Fatalf("WriteTombstone: %v", err)
	}
	tombPath := filepath.Join(cs.ArchiveDir(), "purgeme.yml")
	if _, err := os.Stat(tombPath); err != nil {
		t.Fatalf("tombstone not written: %v", err)
	}

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/purgeme/purge", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204; body=%s", w.Code, w.Body.String())
	}
	if _, err := s.GetAppBySlug("purgeme"); err == nil {
		t.Errorf("DB row still present after purge")
	}
	if _, err := os.Stat(tombPath); !os.IsNotExist(err) {
		t.Errorf("tombstone file still on disk: err=%v", err)
	}
}

func TestDeleteApp_409OnArchived(t *testing.T) {
	srv, _ := newDeployTestServer(t)
	srv.SetReconciler(&mockReconciler{})
	seedArchivedApp(t, srv.store, "gone")

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodDelete, "/api/apps/gone", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409; body=%s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "/purge") {
		t.Errorf("expected hint about /purge in body, got: %s", w.Body.String())
	}
	if _, err := srv.store.GetAppBySlug("gone"); err != nil {
		t.Errorf("archived app should still exist: %v", err)
	}
}

func TestDeleteApp_PurgesNonArchived(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	srv.SetReconciler(&mockReconciler{})
	srv.SetAudit(audit.NewRecorder(srv.store))

	if err := srv.store.UpsertApp(&store.App{
		Name:        "kill",
		Slug:        "kill",
		ComposePath: filepath.Join(appsDir, "kill", "docker-compose.yml"),
		Status:      "running",
	}, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	app, _ := srv.store.GetAppBySlug("kill")

	// Seed history rows that PurgeApp should sweep.
	if err := srv.store.CreateDeployEvent("kill", "deploy", nil, ""); err != nil {
		t.Fatalf("CreateDeployEvent: %v", err)
	}
	if _, err := srv.store.RecordAudit(context.Background(), store.AuditEntry{
		AppID:    &app.ID,
		AppSlug:  "kill",
		Category: "lifecycle",
		Action:   "deployed",
		Summary:  "x",
	}); err != nil {
		t.Fatalf("RecordAudit seed: %v", err)
	}

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodDelete, "/api/apps/kill", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := srv.store.GetAppBySlug("kill"); err == nil {
		t.Errorf("app row still present after delete")
	}
	// Deploy events for the slug should be gone.
	events, err := srv.store.ListDeployEvents("kill")
	if err != nil {
		t.Fatalf("ListDeployEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("history not purged, got %d events", len(events))
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
