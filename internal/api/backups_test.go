package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestCreateBackupConfig(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)

	cookie := superAdminCookie(t, srv.jwt)
	body, _ := json.Marshal(map[string]interface{}{
		"Strategy":         "postgres",
		"Target":           "local",
		"ScheduleCron":     "0 2 * * *",
		"TargetConfigJSON": "",
		"RetentionCount":   5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/backups/configs", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var cfg store.BackupConfig
	json.NewDecoder(w.Body).Decode(&cfg)
	if cfg.ID == 0 {
		t.Error("expected non-zero ID in response")
	}
	if cfg.Strategy != "postgres" {
		t.Errorf("Strategy = %q, want postgres", cfg.Strategy)
	}
}

func TestListBackupConfigs(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	app, _ := s.GetAppBySlug("myapp")
	s.CreateBackupConfig(&store.BackupConfig{
		AppID:          app.ID,
		Strategy:       "postgres",
		Target:         "local",
		ScheduleCron:   "0 2 * * *",
		RetentionCount: 3,
	})

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/backups/configs", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var cfgs []store.BackupConfig
	json.NewDecoder(w.Body).Decode(&cfgs)
	if len(cfgs) != 1 {
		t.Errorf("got %d configs, want 1", len(cfgs))
	}
}

func TestListBackupRuns(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	app, _ := s.GetAppBySlug("myapp")
	cfg := &store.BackupConfig{
		AppID:          app.ID,
		Strategy:       "postgres",
		Target:         "local",
		ScheduleCron:   "0 2 * * *",
		RetentionCount: 3,
	}
	s.CreateBackupConfig(cfg)
	s.CreateBackupRun(cfg.ID)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/backups/runs", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var runs []store.BackupRun
	json.NewDecoder(w.Body).Decode(&runs)
	if len(runs) != 1 {
		t.Errorf("got %d runs, want 1", len(runs))
	}
}

func TestTriggerBackupNoScheduler(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/backups/run", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// no scheduler configured -> 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestDeleteBackupConfig(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	app, _ := s.GetAppBySlug("myapp")
	cfg := &store.BackupConfig{
		AppID:          app.ID,
		Strategy:       "postgres",
		Target:         "local",
		ScheduleCron:   "0 2 * * *",
		RetentionCount: 3,
	}
	s.CreateBackupConfig(cfg)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/backups/configs/%d", cfg.ID), nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}
