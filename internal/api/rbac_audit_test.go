package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

// lockedDownGETs lists routes that should now require super_admin.
var lockedDownGETs = []string{
	"/api/docker/info",
	"/api/docker/disk-usage",
	"/api/docker/images",
	"/api/docker/networks",
	"/api/docker/volumes",
	"/api/system/info",
	"/api/system/storage-breakdown",
	"/api/system/audit-config",
}

// TestRBAC_LockedDownGETs_SuperAdminNot403 ensures super_admin is not blocked
// by the middleware on the locked-down GETs (we don't assert 200 because some
// handlers depend on docker/sysinfo which may 503 in test env; we only assert
// the access control gate doesn't reject).
func TestRBAC_LockedDownGETs_SuperAdminNot403(t *testing.T) {
	srv, _, adminCookie := setupUserTestServer(t)

	for _, path := range lockedDownGETs {
		t.Run("admin "+path, func(t *testing.T) {
			req := authedRequest(t, http.MethodGet, path, nil, adminCookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code == http.StatusForbidden {
				t.Fatalf("super_admin got 403 on %s: %s", path, w.Body.String())
			}
		})
	}
}

func TestRBAC_LockedDownGETs_ManageForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")

	for _, path := range lockedDownGETs {
		t.Run("manage "+path, func(t *testing.T) {
			req := authedRequest(t, http.MethodGet, path, nil, manageCookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("manage got %d on %s, want 403; body: %s", w.Code, path, w.Body.String())
			}
		})
	}
}

func TestRBAC_LockedDownGETs_ViewerForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")

	for _, path := range lockedDownGETs {
		t.Run("viewer "+path, func(t *testing.T) {
			req := authedRequest(t, http.MethodGet, path, nil, viewerCookie)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("viewer got %d on %s, want 403; body: %s", w.Code, path, w.Body.String())
			}
		})
	}
}

func TestRBAC_TestS3_ManageForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	req := authedRequest(t, http.MethodPost, "/api/backups/test-s3", map[string]string{}, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("manage got %d on POST /api/backups/test-s3, want 403", w.Code)
	}
}

func TestRBAC_TestS3_ViewerForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")
	req := authedRequest(t, http.MethodPost, "/api/backups/test-s3", map[string]string{}, viewerCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("viewer got %d on POST /api/backups/test-s3, want 403", w.Code)
	}
}

// seedTwoBackupApps creates two apps "alpha" and "beta", each with a backup
// config and one successful run, returns their app rows.
func seedTwoBackupApps(t *testing.T, st *store.Store) (a *store.App, b *store.App) {
	t.Helper()
	if err := st.UpsertApp(&store.App{Name: "alpha", Slug: "alpha", ComposePath: "/tmp/a.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if err := st.UpsertApp(&store.App{Name: "beta", Slug: "beta", ComposePath: "/tmp/b.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed beta: %v", err)
	}
	a, err := st.GetAppBySlug("alpha")
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}
	b, err = st.GetAppBySlug("beta")
	if err != nil {
		t.Fatalf("get beta: %v", err)
	}
	cfgA := &store.BackupConfig{AppID: a.ID, Strategy: "postgres", Target: "local", ScheduleCron: "0 2 * * *", RetentionMode: "count", RetentionCount: 3}
	if err := st.CreateBackupConfig(cfgA); err != nil {
		t.Fatalf("cfg alpha: %v", err)
	}
	cfgB := &store.BackupConfig{AppID: b.ID, Strategy: "postgres", Target: "local", ScheduleCron: "0 2 * * *", RetentionMode: "count", RetentionCount: 3}
	if err := st.CreateBackupConfig(cfgB); err != nil {
		t.Fatalf("cfg beta: %v", err)
	}
	runA, err := st.CreateBackupRun(cfgA.ID)
	if err != nil {
		t.Fatalf("run alpha: %v", err)
	}
	if err := st.UpdateBackupRunSuccess(runA.ID, 1024, "/tmp/a.dump", "sha256:a"); err != nil {
		t.Fatalf("update run alpha: %v", err)
	}
	runB, err := st.CreateBackupRun(cfgB.ID)
	if err != nil {
		t.Fatalf("run beta: %v", err)
	}
	if err := st.UpdateBackupRunSuccess(runB.ID, 2048, "/tmp/b.dump", "sha256:b"); err != nil {
		t.Fatalf("update run beta: %v", err)
	}
	return a, b
}

func decodeBackupSummary(t *testing.T, w *httptest.ResponseRecorder) (apps []store.BackupSummaryApp, runs []store.BackupRunWithApp) {
	t.Helper()
	var resp struct {
		Apps       []store.BackupSummaryApp `json:"apps"`
		RecentRuns []store.BackupRunWithApp `json:"recent_runs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	return resp.Apps, resp.RecentRuns
}

func TestRBAC_BackupSummary_SuperAdminSeesAll(t *testing.T) {
	srv, st, adminCookie := setupUserTestServer(t)
	seedTwoBackupApps(t, st)

	req := authedRequest(t, http.MethodGet, "/api/backups/summary", nil, adminCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	apps, runs := decodeBackupSummary(t, w)
	if len(apps) != 2 {
		t.Errorf("super_admin saw %d apps, want 2: %+v", len(apps), apps)
	}
	if len(runs) != 2 {
		t.Errorf("super_admin saw %d runs, want 2", len(runs))
	}
}

func TestRBAC_BackupSummary_ManageFiltered(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	a, _ := seedTwoBackupApps(t, st)

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	mgr, _ := st.GetUserByUsername("mgr")
	if err := st.GrantAppAccess(mgr.ID, a.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	req := authedRequest(t, http.MethodGet, "/api/backups/summary", nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	apps, runs := decodeBackupSummary(t, w)
	if len(apps) != 1 || apps[0].AppSlug != "alpha" {
		t.Errorf("manage apps = %+v, want only alpha", apps)
	}
	for _, r := range runs {
		if r.AppSlug != "alpha" {
			t.Errorf("manage saw run for %s, want only alpha", r.AppSlug)
		}
	}
}

func TestRBAC_BackupSummary_ViewerFiltered(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	_, b := seedTwoBackupApps(t, st)

	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")
	v, _ := st.GetUserByUsername("v1")
	if err := st.GrantAppAccess(v.ID, b.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	req := authedRequest(t, http.MethodGet, "/api/backups/summary", nil, viewerCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	apps, runs := decodeBackupSummary(t, w)
	if len(apps) != 1 || apps[0].AppSlug != "beta" {
		t.Errorf("viewer apps = %+v, want only beta", apps)
	}
	for _, r := range runs {
		if r.AppSlug != "beta" {
			t.Errorf("viewer saw run for %s, want only beta", r.AppSlug)
		}
	}
}

func TestRBAC_BackupSummary_NoGrantEmpty(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	seedTwoBackupApps(t, st)

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	req := authedRequest(t, http.MethodGet, "/api/backups/summary", nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	apps, runs := decodeBackupSummary(t, w)
	if len(apps) != 0 {
		t.Errorf("no-grant apps = %+v, want empty", apps)
	}
	if len(runs) != 0 {
		t.Errorf("no-grant runs = %+v, want empty", runs)
	}
}

// --- Archived list filtering ---

func seedTwoArchived(t *testing.T, st *store.Store) (alphaID, betaID int64) {
	t.Helper()
	if err := st.UpsertApp(&store.App{Name: "alpha", Slug: "alpha", ComposePath: "/tmp/a.yml", Status: "stopped"}, nil); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if err := st.UpsertApp(&store.App{Name: "beta", Slug: "beta", ComposePath: "/tmp/b.yml", Status: "stopped"}, nil); err != nil {
		t.Fatalf("seed beta: %v", err)
	}
	if err := st.MarkAppArchived("alpha", time.Now()); err != nil {
		t.Fatalf("archive alpha: %v", err)
	}
	if err := st.MarkAppArchived("beta", time.Now()); err != nil {
		t.Fatalf("archive beta: %v", err)
	}
	a, _ := st.GetAppBySlug("alpha")
	b, _ := st.GetAppBySlug("beta")
	return a.ID, b.ID
}

func TestRBAC_Archived_SuperAdminSeesAll(t *testing.T) {
	srv, st, adminCookie := setupUserTestServer(t)
	seedTwoArchived(t, st)

	req := authedRequest(t, http.MethodGet, "/api/apps/archived", nil, adminCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Errorf("super_admin saw %d archived, want 2", len(resp))
	}
}

func TestRBAC_Archived_ManageFiltered(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	alphaID, _ := seedTwoArchived(t, st)

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	mgr, _ := st.GetUserByUsername("mgr")
	if err := st.GrantAppAccess(mgr.ID, alphaID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	req := authedRequest(t, http.MethodGet, "/api/apps/archived", nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 || resp[0]["Slug"] != "alpha" {
		t.Errorf("manage archived = %+v, want only alpha", resp)
	}
}

func TestRBAC_Archived_ViewerFiltered(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	_, betaID := seedTwoArchived(t, st)

	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")
	v, _ := st.GetUserByUsername("v1")
	if err := st.GrantAppAccess(v.ID, betaID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	req := authedRequest(t, http.MethodGet, "/api/apps/archived", nil, viewerCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 || resp[0]["Slug"] != "beta" {
		t.Errorf("viewer archived = %+v, want only beta", resp)
	}
}

func TestRBAC_Archived_NoGrantEmpty(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	seedTwoArchived(t, st)

	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	req := authedRequest(t, http.MethodGet, "/api/apps/archived", nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("no-grant archived = %+v, want empty", resp)
	}
}
