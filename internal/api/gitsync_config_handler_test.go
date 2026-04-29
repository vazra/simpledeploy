package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/config"
)

func gitPlainInit(path string, bare bool) (*git.Repository, error) {
	return git.PlainInit(path, bare)
}

func TestGetGitConfig_Unauth(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/git/config", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestGetGitConfig_NonSuperAdmin(t *testing.T) {
	srv, st := newTestServer(t)
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	if _, err := st.CreateUser("regular", "hashed", "manage", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	tok, _ := jwtMgr.Generate(2, "regular", "admin")
	req := httptest.NewRequest("GET", "/api/git/config", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tok})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

func TestGetGitConfig_EmptyDB_YAMLFallback(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg = &config.Config{
		GitSync: config.GitSyncConfig{
			Enabled: false,
		},
	}
	srv.masterSecret = "test-master-secret"

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	cookie := superAdminCookie(t, jwtMgr)

	req := httptest.NewRequest("GET", "/api/git/config", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp gitConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Enabled {
		t.Error("expected Enabled=false")
	}
	if resp.Source != "yaml" {
		t.Errorf("expected source=yaml, got %q", resp.Source)
	}
}

func TestPutGitConfig_SetAndGet(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg = &config.Config{
		AppsDir:      t.TempDir(),
		MasterSecret: "test-master-secret",
	}
	srv.masterSecret = "test-master-secret"

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	cookie := superAdminCookie(t, jwtMgr)

	// We won't actually start the syncer here (no git remote), so just verify
	// the config is written to the DB correctly.
	secret := "my-webhook-secret"
	payload := gitConfigRequest{
		Enabled:             false, // disabled so ReloadGitSync won't try to connect
		Remote:              "file:///tmp/bare.git",
		Branch:              "main",
		PollIntervalSeconds: 30,
		WebhookSecret:       &secret,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("PUT", "/api/git/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("PUT want 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the DB has the values.
	kv, err := srv.store.GetGitSyncConfig()
	if err != nil {
		t.Fatalf("GetGitSyncConfig: %v", err)
	}
	if kv["remote"] != "file:///tmp/bare.git" {
		t.Errorf("remote in DB: %q", kv["remote"])
	}
	if kv["poll_interval"] != "30" {
		t.Errorf("poll_interval in DB: %q", kv["poll_interval"])
	}
	// Secret should be stored encrypted (non-empty, not the plaintext).
	if kv["webhook_secret_enc"] == "" {
		t.Error("webhook_secret_enc should be non-empty after setting")
	}
	if kv["webhook_secret_enc"] == secret {
		t.Error("webhook_secret_enc should not be stored as plaintext")
	}

	// GET should show webhook_secret_set=true without exposing the value.
	req2 := httptest.NewRequest("GET", "/api/git/config", nil)
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	var resp gitConfigResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if !resp.WebhookSecretSet {
		t.Error("expected webhook_secret_set=true after PUT")
	}
	if resp.Source != "db" {
		t.Errorf("expected source=db, got %q", resp.Source)
	}
}

func TestPutGitConfig_EmptyRemote_AllowsSave(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg = &config.Config{
		AppsDir:      t.TempDir(),
		MasterSecret: "test-master-secret",
	}
	srv.masterSecret = "test-master-secret"

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	cookie := superAdminCookie(t, jwtMgr)

	// Brand-new empty bare repo (no branches, no refs).
	bareDir := t.TempDir() + "/empty.git"
	if _, err := gitPlainInit(bareDir, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}

	payload := gitConfigRequest{
		Enabled:             true,
		Remote:              bareDir,
		Branch:              "main",
		AuthorName:          "test",
		AuthorEmail:         "test@example.com",
		PollIntervalSeconds: 30,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("PUT", "/api/git/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT want 200, got %d: %s", w.Code, w.Body.String())
	}

	kv, err := srv.store.GetGitSyncConfig()
	if err != nil {
		t.Fatalf("GetGitSyncConfig: %v", err)
	}
	if kv["remote"] != bareDir {
		t.Errorf("remote in DB: %q want %q", kv["remote"], bareDir)
	}
	if kv["enabled"] != "true" {
		t.Errorf("enabled in DB: %q", kv["enabled"])
	}
}

func TestPutGitConfig_Validation(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg = &config.Config{}
	srv.masterSecret = "secret"

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	cookie := superAdminCookie(t, jwtMgr)

	// enabled=true without remote.
	payload := gitConfigRequest{Enabled: true, Remote: ""}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("PUT", "/api/git/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 when enabled without remote, got %d", w.Code)
	}

	// poll_interval too small.
	payload2 := gitConfigRequest{Enabled: false, PollIntervalSeconds: 2}
	body2, _ := json.Marshal(payload2)
	req2 := httptest.NewRequest("PUT", "/api/git/config", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Errorf("want 400 for poll_interval<5, got %d", w2.Code)
	}
}

func TestPutGitConfig_PersistsToggles(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg = &config.Config{
		AppsDir:      t.TempDir(),
		MasterSecret: "test-master-secret",
	}
	srv.masterSecret = "test-master-secret"

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	cookie := superAdminCookie(t, jwtMgr)

	f := false
	payload := gitConfigRequest{
		Enabled:          false,
		Remote:           "file:///tmp/bare.git",
		PollEnabled:      &f,
		AutoPushEnabled:  &f,
		AutoApplyEnabled: &f,
		WebhookEnabled:   &f,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("PUT", "/api/git/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT want 200, got %d: %s", w.Code, w.Body.String())
	}

	kv, err := srv.store.GetGitSyncConfig()
	if err != nil {
		t.Fatalf("GetGitSyncConfig: %v", err)
	}
	for _, key := range []string{"poll_enabled", "auto_push_enabled", "auto_apply_enabled", "webhook_enabled"} {
		if kv[key] != "false" {
			t.Errorf("expected %s=false in DB, got %q", key, kv[key])
		}
	}

	// GET should reflect the toggles.
	req2 := httptest.NewRequest("GET", "/api/git/config", nil)
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	var resp gitConfigResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if resp.PollEnabled || resp.AutoPushEnabled || resp.AutoApplyEnabled || resp.WebhookEnabled {
		t.Errorf("toggles should all be false: poll=%v push=%v apply=%v webhook=%v",
			resp.PollEnabled, resp.AutoPushEnabled, resp.AutoApplyEnabled, resp.WebhookEnabled)
	}
}

func TestDisableGitSync(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg = &config.Config{AppsDir: t.TempDir(), MasterSecret: "secret"}
	srv.masterSecret = "secret"

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	cookie := superAdminCookie(t, jwtMgr)

	// Put something in the DB first.
	_ = srv.store.SetGitSyncConfig(map[string]string{"enabled": "true", "remote": "file:///tmp/bare.git"})

	req := httptest.NewRequest("POST", "/api/git/disable", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	// DB should be empty now.
	kv, _ := srv.store.GetGitSyncConfig()
	if len(kv) != 0 {
		t.Errorf("expected empty DB after disable, got %v", kv)
	}
}
