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
		"strategy":           "postgres",
		"target":             "local",
		"schedule_cron":      "0 2 * * *",
		"target_config_json": "",
		"retention_mode":     "count",
		"retention_count":    5,
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
		RetentionMode:  "count",
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
		RetentionMode:  "count",
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
		RetentionMode:  "count",
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

func TestBackupSummary(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	app, _ := s.GetAppBySlug("myapp")
	cfg := &store.BackupConfig{
		AppID:          app.ID,
		Strategy:       "postgres",
		Target:         "local",
		ScheduleCron:   "0 2 * * *",
		RetentionMode:  "count",
		RetentionCount: 3,
	}
	s.CreateBackupConfig(cfg)
	s.CreateBackupRun(cfg.ID)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodGet, "/api/backups/summary", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Apps       []interface{} `json:"apps"`
		RecentRuns []interface{} `json:"recent_runs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Apps) == 0 {
		t.Error("expected at least one app in summary")
	}
	if resp.RecentRuns == nil {
		t.Error("expected recent_runs to be non-nil (even if empty)")
	}
}

// --- RBAC tests for /api/backups/configs/{id} (PUT/DELETE) ---

// seedBackupConfigForApp creates an app + a backup config and returns the cfg ID.
func seedBackupConfigForApp(t *testing.T, st *store.Store, slug string) int64 {
	t.Helper()
	if err := st.UpsertApp(&store.App{Name: slug, Slug: slug, ComposePath: "/tmp/" + slug + ".yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app %s: %v", slug, err)
	}
	app, err := st.GetAppBySlug(slug)
	if err != nil {
		t.Fatalf("get app %s: %v", slug, err)
	}
	cfg := &store.BackupConfig{
		AppID:          app.ID,
		Strategy:       "postgres",
		Target:         "local",
		ScheduleCron:   "0 2 * * *",
		RetentionMode:  "count",
		RetentionCount: 3,
	}
	if err := st.CreateBackupConfig(cfg); err != nil {
		t.Fatalf("create cfg: %v", err)
	}
	return cfg.ID
}

func TestUpdateBackupConfig_ManageWithGrant(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")
	app, _ := st.GetAppBySlug("alpha")

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	mgr, _ := st.GetUserByUsername("mgr")
	if err := st.GrantAppAccess(mgr.ID, app.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	body := map[string]any{
		"strategy":         "postgres",
		"target":           "local",
		"schedule_cron":    "0 3 * * *",
		"retention_mode":   "count",
		"retention_count":  7,
	}
	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", cfgID), body, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateBackupConfig_ManageWithoutGrant(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")

	body := map[string]any{"strategy": "postgres", "target": "local"}
	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", cfgID), body, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestUpdateBackupConfig_ViewerForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")
	app, _ := st.GetAppBySlug("alpha")

	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")
	v, _ := st.GetUserByUsername("v1")
	if err := st.GrantAppAccess(v.ID, app.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	body := map[string]any{"strategy": "postgres", "target": "local"}
	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", cfgID), body, viewerCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestUpdateBackupConfig_SuperAdmin(t *testing.T) {
	srv, st, adminCookie := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")

	body := map[string]any{
		"strategy":        "postgres",
		"target":          "local",
		"schedule_cron":   "0 4 * * *",
		"retention_mode":  "count",
		"retention_count": 9,
	}
	req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", cfgID), body, adminCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteBackupConfig_ManageWithGrant(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")
	app, _ := st.GetAppBySlug("alpha")

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	mgr, _ := st.GetUserByUsername("mgr")
	if err := st.GrantAppAccess(mgr.ID, app.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/backups/configs/%d", cfgID), nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteBackupConfig_ManageWithoutGrant(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")

	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/backups/configs/%d", cfgID), nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestDeleteBackupConfig_ViewerForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	cfgID := seedBackupConfigForApp(t, st, "alpha")
	app, _ := st.GetAppBySlug("alpha")

	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")
	v, _ := st.GetUserByUsername("v1")
	if err := st.GrantAppAccess(v.ID, app.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/backups/configs/%d", cfgID), nil, viewerCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestUpdateBackupConfig_NotFound(t *testing.T) {
	srv, _, adminCookie := setupUserTestServer(t)
	body := map[string]any{"strategy": "postgres", "target": "local"}
	req := authedRequest(t, http.MethodPut, "/api/backups/configs/99999", body, adminCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestTriggerBackupConfig(t *testing.T) {
	srv, s := newTestServer(t)
	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	app, _ := s.GetAppBySlug("myapp")
	cfg := &store.BackupConfig{
		AppID:          app.ID,
		Strategy:       "postgres",
		Target:         "local",
		ScheduleCron:   "0 2 * * *",
		RetentionMode:  "count",
		RetentionCount: 3,
	}
	s.CreateBackupConfig(cfg)

	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/run", cfg.ID), nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// no scheduler configured -> 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
