package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/store"
)

// newHooksTestServer registers app "myapp" with a compose file holding
// services web and db, plus the postgres strategy (which runs inside db).
func newHooksTestServer(t *testing.T) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	srv, st := newTestServer(t)
	composePath := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: nginx\n  db:\n    image: postgres:16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	sched := backup.NewScheduler(st, nil)
	sched.RegisterStrategy("postgres", backup.NewPostgresStrategy())
	sched.RegisterStrategy("volume", backup.NewVolumeStrategy())
	srv.SetBackupScheduler(sched)
	return srv, st, superAdminCookie(t, srv.jwt)
}

func backupCfgBody(pre, post string) map[string]any {
	return map[string]any{
		"strategy":        "postgres",
		"target":          "local",
		"schedule_cron":   "0 2 * * *",
		"retention_mode":  "count",
		"retention_count": 5,
		"pre_hooks":       pre,
		"post_hooks":      post,
	}
}

func postBackupCfg(t *testing.T, srv *Server, cookie *http.Cookie, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/backups/configs", bytes.NewReader(b))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestCreateBackupConfigHooksValid(t *testing.T) {
	srv, st, cookie := newHooksTestServer(t)
	w := postBackupCfg(t, srv, cookie, backupCfgBody(
		`[{"type":"stop","service":" web "},{"type":"exec","service":"db","command":"sync","timeout":60}]`,
		`[{"type":"start","service":"web"}]`))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var resp store.BackupConfig
	json.NewDecoder(w.Body).Decode(&resp)
	stored, err := st.GetBackupConfig(resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"type":"stop","service":"web"},{"type":"exec","service":"db","command":"sync","timeout":60}]`; stored.PreHooks != want {
		t.Errorf("stored pre_hooks = %s, want %s", stored.PreHooks, want)
	}
	if want := `[{"type":"start","service":"web"}]`; stored.PostHooks != want {
		t.Errorf("stored post_hooks = %s, want %s", stored.PostHooks, want)
	}
}

func TestCreateBackupConfigHooksRejected(t *testing.T) {
	long := strings.Repeat("x", backup.MaxHookCommandLen+1)
	cases := map[string]struct{ pre, post, want string }{
		"unknown type":       {`[{"type":"docker_rm","service":"web"}]`, "", "unknown type"},
		"other app service":  {`[{"type":"stop","service":"cache"}]`, "", "not a service of this app"},
		"raw container name": {"", `[{"type":"exec","service":"simpledeploy-other-db-1","command":"id"}]`, "not a service of this app"},
		"simpledeploy":       {`[{"type":"stop","service":"simpledeploy"}]`, "", "not a service of this app"},
		"no service":         {`[{"type":"stop"}]`, "", "choose the service"},
		"exec no command":    {`[{"type":"exec","service":"web"}]`, "", "enter the command"},
		"command too long":   {fmt.Sprintf(`[{"type":"exec","service":"web","command":%q}]`, long), "", "longer than"},
		"timeout too long":   {`[{"type":"stop","service":"web","timeout":999999}]`, "", "timeout must be between"},
		"negative timeout":   {"", `[{"type":"start","service":"web","timeout":-5}]`, "timeout must be between"},
		"not a list":         {`{"type":"stop"}`, "", "JSON list"},
		"stops backup svc":   {`[{"type":"stop","service":"db"}]`, "", "cannot be stopped"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, st, cookie := newHooksTestServer(t)
			w := postBackupCfg(t, srv, cookie, backupCfgBody(tc.pre, tc.post))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("body = %q, want it to contain %q", w.Body.String(), tc.want)
			}
			app, _ := st.GetAppBySlug("myapp")
			if cfgs, _ := st.ListBackupConfigs(&app.ID); len(cfgs) != 0 {
				t.Fatalf("config stored despite 400: %+v", cfgs)
			}
		})
	}
}

func TestCreateBackupConfigHooksNeedComposeFile(t *testing.T) {
	srv, st := newTestServer(t)
	st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: filepath.Join(t.TempDir(), "missing.yml"), Status: "running"}, nil)
	cookie := superAdminCookie(t, srv.jwt)

	w := postBackupCfg(t, srv, cookie, backupCfgBody(`[{"type":"stop","service":"web"}]`, ""))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "compose file") {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	// No hooks: the compose file is not needed.
	w = postBackupCfg(t, srv, cookie, backupCfgBody("[]", ""))
	if w.Code != http.StatusCreated {
		t.Fatalf("no hooks: status = %d, body: %s", w.Code, w.Body.String())
	}
}

func TestCreateBackupConfigDropsLegacyWizardHooks(t *testing.T) {
	srv, st, cookie := newHooksTestServer(t)
	w := postBackupCfg(t, srv, cookie, backupCfgBody(
		`[{"type":"stop_container","service":{"service_name":"db","container_name":"simpledeploy-myapp-db-1"}}]`,
		`[{"type":"start_container","service":{"service_name":"db"}}]`))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var resp store.BackupConfig
	json.NewDecoder(w.Body).Decode(&resp)
	stored, _ := st.GetBackupConfig(resp.ID)
	if stored.PreHooks != "[]" || stored.PostHooks != "[]" {
		t.Fatalf("stored hooks = %q / %q, want []", stored.PreHooks, stored.PostHooks)
	}
}

func TestUpdateBackupConfigHooksValidated(t *testing.T) {
	srv, st, cookie := newHooksTestServer(t)
	app, _ := st.GetAppBySlug("myapp")
	cfg := &store.BackupConfig{AppID: app.ID, Strategy: "postgres", Target: "local", ScheduleCron: "0 2 * * *", RetentionMode: "count", RetentionCount: 3}
	if err := st.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	put := func(body map[string]any) *httptest.ResponseRecorder {
		req := authedRequest(t, http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", cfg.ID), body, cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	w := put(backupCfgBody(`[{"type":"exec","service":"redis","command":"x"}]`, ""))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Services in this app: db, web") {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	if got, _ := st.GetBackupConfig(cfg.ID); got.PreHooks != "" {
		t.Fatalf("invalid hooks stored: %q", got.PreHooks)
	}

	w = put(backupCfgBody(`[{"type":"flush_redis","service":"web"}]`, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	if got, _ := st.GetBackupConfig(cfg.ID); got.PreHooks != `[{"type":"flush_redis","service":"web"}]` {
		t.Fatalf("stored pre_hooks = %q", got.PreHooks)
	}
}

// The wizard sends retention_mode "days"; the database only allows
// "count" and "time".
func TestCreateBackupConfigRetentionMode(t *testing.T) {
	cases := []struct {
		mode     string
		wantCode int
		wantMode string
	}{
		{"days", http.StatusCreated, "time"},
		{"time", http.StatusCreated, "time"},
		{"count", http.StatusCreated, "count"},
		{"", http.StatusCreated, "count"},
		{"weekly", http.StatusBadRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			srv, st, cookie := newHooksTestServer(t)
			body := backupCfgBody("", "")
			body["retention_mode"] = tc.mode
			body["retention_days"] = 14
			w := postBackupCfg(t, srv, cookie, body)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tc.wantCode, w.Body.String())
			}
			if tc.wantMode == "" {
				return
			}
			var resp store.BackupConfig
			json.NewDecoder(w.Body).Decode(&resp)
			stored, _ := st.GetBackupConfig(resp.ID)
			if stored.RetentionMode != tc.wantMode {
				t.Fatalf("stored retention_mode = %q, want %q", stored.RetentionMode, tc.wantMode)
			}
		})
	}
}

// Command-running hooks (exec, flush) are super admin only; a manage user
// may use stop/start and may edit a config whose command hooks they leave
// unchanged.
func TestBackupCommandHooksSuperAdminOnly(t *testing.T) {
	srv, st, admin := newHooksTestServer(t)
	app, err := st.GetAppBySlug("myapp")
	if err != nil {
		t.Fatal(err)
	}
	mgr := makeUserCookie(t, srv, st, "mgr", app.ID)
	execHook := `[{"type":"exec","service":"db","command":"sync"}]`

	if w := postBackupCfg(t, srv, mgr, backupCfgBody(execHook, "")); w.Code != http.StatusForbidden {
		t.Fatalf("manage create with exec hook: status = %d, want 403; body: %s", w.Code, w.Body.String())
	}
	if w := postBackupCfg(t, srv, mgr, backupCfgBody(`[{"type":"stop","service":"web"}]`, `[{"type":"start","service":"web"}]`)); w.Code != http.StatusCreated {
		t.Fatalf("manage create with stop/start: status = %d, body: %s", w.Code, w.Body.String())
	}

	w := postBackupCfg(t, srv, admin, backupCfgBody(execHook, ""))
	if w.Code != http.StatusCreated {
		t.Fatalf("admin create with exec hook: status = %d, body: %s", w.Code, w.Body.String())
	}
	var created store.BackupConfig
	json.NewDecoder(w.Body).Decode(&created)

	put := func(body map[string]any) int {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", created.ID), bytes.NewReader(b))
		req.AddCookie(mgr)
		rw := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rw, req)
		return rw.Code
	}
	keep := backupCfgBody(execHook, "")
	keep["retention_count"] = 9
	if code := put(keep); code != http.StatusOK {
		t.Fatalf("manage edit keeping admin's exec hook: status = %d, want 200", code)
	}
	if code := put(backupCfgBody(`[{"type":"exec","service":"db","command":"id"}]`, "")); code != http.StatusForbidden {
		t.Fatalf("manage changing exec command: status = %d, want 403", code)
	}
	if code := put(backupCfgBody(execHook, `[{"type":"flush_redis","service":"db"}]`)); code != http.StatusForbidden {
		t.Fatalf("manage adding a flush hook: status = %d, want 403", code)
	}
	stored, err := st.GetBackupConfig(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PreHooks != execHook || stored.RetentionCount != 9 {
		t.Fatalf("stored = pre %s retention %d, want admin's hook kept and retention 9", stored.PreHooks, stored.RetentionCount)
	}
	if code := put(backupCfgBody("", "")); code != http.StatusOK {
		t.Fatalf("manage removing the exec hook: status = %d, want 200", code)
	}
}
