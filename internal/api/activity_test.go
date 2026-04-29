package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

// seedAudit inserts an AuditEntry and returns its id.
func seedAudit(t *testing.T, st *store.Store, e store.AuditEntry) int64 {
	t.Helper()
	id, err := st.RecordAudit(context.Background(), e)
	if err != nil {
		t.Fatalf("seed audit: %v", err)
	}
	return id
}

// makeAdminCookie issues a JWT for user ID 1 (the "admin" super_admin created by newTestServer).
func makeAdminCookie(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	return superAdminCookie(t, srv.jwt)
}

// makeUserCookie creates a regular user + optional app access, logs in, returns cookie.
func makeUserCookie(t *testing.T, srv *Server, st *store.Store, username string, appIDs ...int64) *http.Cookie {
	t.Helper()
	hash, err := auth.HashPassword("pass")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(username, hash, "manage", "", "")
	if err != nil {
		t.Fatalf("create user %q: %v", username, err)
	}
	for _, aid := range appIDs {
		if err := st.GrantAppAccess(u.ID, aid); err != nil {
			t.Fatalf("grant access: %v", err)
		}
	}
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": username, "password": "pass",
	})
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	t.Fatalf("no session cookie for %q", username)
	return nil
}

// seedApp creates a minimal app in the store and returns its ID.
func seedApp(t *testing.T, st *store.Store, slug string) int64 {
	t.Helper()
	app := &store.App{Name: slug, Slug: slug, ComposePath: fmt.Sprintf("/tmp/%s.yml", slug), Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app %q: %v", slug, err)
	}
	got, err := st.GetAppBySlug(slug)
	if err != nil {
		t.Fatalf("get app %q: %v", slug, err)
	}
	return got.ID
}

func doRequest(t *testing.T, srv *Server, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// TestActivityListGlobal_Admin verifies admin sees entries from all apps + system entries.
func TestActivityListGlobal_Admin(t *testing.T) {
	srv, st := newTestServer(t)
	appID := seedApp(t, st, "myapp")
	uid := int64(1)

	seedAudit(t, st, store.AuditEntry{AppID: &appID, AppSlug: "myapp", Category: "deploy", Action: "deploy", Summary: "app entry"})
	seedAudit(t, st, store.AuditEntry{ActorUserID: &uid, Category: "auth", Action: "login", Summary: "system entry"})

	cookie := makeAdminCookie(t, srv)
	w := doRequest(t, srv, http.MethodGet, "/api/activity", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries := resp["entries"].([]any)
	if len(entries) < 2 {
		t.Errorf("admin should see at least 2 entries, got %d", len(entries))
	}
}

// TestActivityListScopedNonAdmin verifies non-admin sees only entries for their apps + own auth events.
// System events and other users' auth events must be invisible.
func TestActivityListScopedNonAdmin(t *testing.T) {
	srv, st := newTestServer(t)
	appA := seedApp(t, st, "app-a")
	appB := seedApp(t, st, "app-b")
	adminUID := int64(1)

	// Create user1 first so we can get its ID for auth event seeding.
	hash, err := auth.HashPassword("pass")
	if err != nil {
		t.Fatal(err)
	}
	user1, err := st.CreateUser("user1", hash, "manage", "", "")
	if err != nil {
		t.Fatalf("create user1: %v", err)
	}
	if err := st.GrantAppAccess(user1.ID, appA); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	seedAudit(t, st, store.AuditEntry{AppID: &appA, AppSlug: "app-a", Category: "deploy", Action: "deploy", Summary: "app-a entry"})
	seedAudit(t, st, store.AuditEntry{AppID: &appB, AppSlug: "app-b", Category: "deploy", Action: "deploy", Summary: "app-b entry"})
	// Auth event belonging to user1 (should be visible to user1).
	seedAudit(t, st, store.AuditEntry{ActorUserID: &user1.ID, Category: "auth", Action: "login", Summary: "user1 auth entry"})
	// Auth event belonging to admin (should NOT be visible to user1).
	seedAudit(t, st, store.AuditEntry{ActorUserID: &adminUID, Category: "auth", Action: "login", Summary: "admin auth entry"})
	// System event (should NOT be visible to user1).
	seedAudit(t, st, store.AuditEntry{Category: "system", Action: "start", Summary: "system entry"})

	// Log in as user1.
	w := postJSON(t, srv, "/api/auth/login", map[string]string{"username": "user1", "password": "pass"})
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie for user1")
	}

	resp := doRequest(t, srv, http.MethodGet, "/api/activity", cookie)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	entries := body["entries"].([]any)

	var summaries []string
	for _, raw := range entries {
		e := raw.(map[string]any)
		summaries = append(summaries, e["summary"].(string))
	}

	// Must see app-a entry and own auth event.
	mustSee := []string{"app-a entry", "user1 auth entry"}
	for _, want := range mustSee {
		found := false
		for _, s := range summaries {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("user1 should see %q; got: %v", want, summaries)
		}
	}

	// Must NOT see app-b, admin auth, or system events.
	mustNotSee := []string{"app-b entry", "admin auth entry", "system entry"}
	for _, bad := range mustNotSee {
		for _, s := range summaries {
			if s == bad {
				t.Errorf("user1 should not see %q; got: %v", bad, summaries)
			}
		}
	}
}

// TestActivityCategoryFilter verifies the categories filter narrows results.
func TestActivityCategoryFilter(t *testing.T) {
	srv, st := newTestServer(t)
	uid := int64(1)

	seedAudit(t, st, store.AuditEntry{ActorUserID: &uid, Category: "deploy", Action: "deploy", Summary: "deploy entry"})
	seedAudit(t, st, store.AuditEntry{ActorUserID: &uid, Category: "auth", Action: "login", Summary: "auth entry"})

	cookie := makeAdminCookie(t, srv)
	w := doRequest(t, srv, http.MethodGet, "/api/activity?categories=auth", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries := resp["entries"].([]any)
	for _, raw := range entries {
		e := raw.(map[string]any)
		if e["category"] != "auth" {
			t.Errorf("expected only auth entries, got category %q", e["category"])
		}
	}
}

// TestActivityPaginationCursor verifies before= cursor returns earlier entries.
func TestActivityPaginationCursor(t *testing.T) {
	srv, st := newTestServer(t)
	uid := int64(1)

	var ids []int64
	for i := 0; i < 5; i++ {
		id := seedAudit(t, st, store.AuditEntry{ActorUserID: &uid, Category: "auth", Action: "login", Summary: fmt.Sprintf("entry-%d", i)})
		ids = append(ids, id)
	}
	// newest id is ids[4]; request before=ids[2] should return ids[1] and ids[0]
	before := ids[2]
	cookie := makeAdminCookie(t, srv)
	w := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/activity?before=%d&limit=10", before), cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries := resp["entries"].([]any)
	for _, raw := range entries {
		e := raw.(map[string]any)
		gotID := int64(e["id"].(float64))
		if gotID >= before {
			t.Errorf("cursor: got entry id %d >= before %d", gotID, before)
		}
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries before cursor, got %d", len(entries))
	}
}

// TestAppActivityForbidden verifies non-admin querying an inaccessible app gets 404.
func TestAppActivityForbidden(t *testing.T) {
	srv, st := newTestServer(t)
	seedApp(t, st, "secret-app")

	// user has no access to secret-app
	cookie := makeUserCookie(t, srv, st, "noaccess")
	w := doRequest(t, srv, http.MethodGet, "/api/apps/secret-app/activity", cookie)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestGetActivityIncludesBeforeAfter verifies single-entry endpoint returns BeforeJSON/AfterJSON.
func TestGetActivityIncludesBeforeAfter(t *testing.T) {
	srv, st := newTestServer(t)
	uid := int64(1)

	id := seedAudit(t, st, store.AuditEntry{
		ActorUserID: &uid,
		Category:    "config",
		Action:      "update",
		Summary:     "config change",
		BeforeJSON:  []byte(`{"key":"old"}`),
		AfterJSON:   []byte(`{"key":"new"}`),
	})

	cookie := makeAdminCookie(t, srv)
	w := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/activity/%d", id), cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var e map[string]any
	json.NewDecoder(w.Body).Decode(&e)

	// BeforeJSON and AfterJSON should be present (non-nil)
	if e["before_json"] == nil {
		t.Error("BeforeJSON should be present in single-entry response")
	}
	if e["after_json"] == nil {
		t.Error("AfterJSON should be present in single-entry response")
	}
}

// TestPurgeAuditSuperAdminOnly verifies non-super-admin gets 403; super-admin succeeds.
func TestPurgeAuditSuperAdminOnly(t *testing.T) {
	srv, st := newTestServer(t)
	uid := int64(1)
	seedAudit(t, st, store.AuditEntry{ActorUserID: &uid, Category: "auth", Action: "login", Summary: "entry"})

	// non-super-admin (role=admin) should get 403
	nonSuperCookie := makeUserCookie(t, srv, st, "regular-admin")
	req := httptest.NewRequest(http.MethodDelete, "/api/activity", nil)
	req.AddCookie(nonSuperCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-super-admin: expected 403, got %d", w.Code)
	}

	// super_admin should succeed
	cookie := makeAdminCookie(t, srv)
	req2 := httptest.NewRequest(http.MethodDelete, "/api/activity", nil)
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusNoContent {
		t.Errorf("super_admin: expected 204, got %d; body: %s", w2.Code, w2.Body.String())
	}

	// list should now be empty
	wList := doRequest(t, srv, http.MethodGet, "/api/activity", cookie)
	var resp map[string]any
	json.NewDecoder(wList.Body).Decode(&resp)
	entries := resp["entries"].([]any)
	if len(entries) != 0 {
		t.Errorf("after purge, expected 0 entries, got %d", len(entries))
	}
}

// TestAuditConfigGetSetSuperAdminOnly verifies both GET and PUT require super-admin.
func TestAuditConfigGetSetSuperAdminOnly(t *testing.T) {
	srv, st := newTestServer(t)

	// non-super-admin GET => 403
	nonSuperCookie := makeUserCookie(t, srv, st, "cfg-admin")
	w := doRequest(t, srv, http.MethodGet, "/api/system/audit-config", nonSuperCookie)
	if w.Code != http.StatusForbidden {
		t.Errorf("non-super GET audit-config: expected 403, got %d", w.Code)
	}

	// super_admin GET => 200
	adminCookie := makeAdminCookie(t, srv)
	wAdminGet := doRequest(t, srv, http.MethodGet, "/api/system/audit-config", adminCookie)
	if wAdminGet.Code != http.StatusOK {
		t.Errorf("super GET audit-config: expected 200, got %d", wAdminGet.Code)
	}
	var cfg map[string]any
	json.NewDecoder(wAdminGet.Body).Decode(&cfg)
	if cfg["retention_days"] == nil {
		t.Error("retention_days missing from response")
	}

	// non-super-admin PUT => 403
	req := httptest.NewRequest(http.MethodPut, "/api/system/audit-config",
		strings.NewReader(`{"retention_days":90}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(nonSuperCookie)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req)
	if w2.Code != http.StatusForbidden {
		t.Errorf("non-super PUT: expected 403, got %d", w2.Code)
	}

	// super_admin PUT => 204
	req3 := httptest.NewRequest(http.MethodPut, "/api/system/audit-config",
		strings.NewReader(`{"retention_days":180}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.AddCookie(adminCookie)
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusNoContent {
		t.Errorf("super PUT: expected 204, got %d; body: %s", w3.Code, w3.Body.String())
	}

	// verify persisted
	days, _ := st.GetAuditRetentionDays(context.Background())
	if days != 180 {
		t.Errorf("expected 180 days, got %d", days)
	}
}

// TestAuditConfigRetentionZero verifies PUT retention_days=0 is accepted (means "forever").
func TestAuditConfigRetentionZero(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := makeAdminCookie(t, srv)

	req := httptest.NewRequest(http.MethodPut, "/api/system/audit-config",
		strings.NewReader(`{"retention_days":0}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT retention_days=0: expected 204, got %d; body: %s", w.Code, w.Body.String())
	}

	days, err := st.GetAuditRetentionDays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Errorf("expected retention_days=0, got %d", days)
	}

	// GET should also return 0.
	wGet := doRequest(t, srv, http.MethodGet, "/api/system/audit-config", cookie)
	if wGet.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d", wGet.Code)
	}
	var cfg map[string]any
	json.NewDecoder(wGet.Body).Decode(&cfg)
	if cfg["retention_days"].(float64) != 0 {
		t.Errorf("GET retention_days: expected 0, got %v", cfg["retention_days"])
	}
}

// TestListActivityAppSlugForbiddenNonAdmin verifies non-admin querying an
// inaccessible app slug via ?app= gets 403.
func TestListActivityAppSlugForbiddenNonAdmin(t *testing.T) {
	srv, st := newTestServer(t)
	seedApp(t, st, "hidden-app")

	// user with no app access
	cookie := makeUserCookie(t, srv, st, "noapp-user")
	w := doRequest(t, srv, http.MethodGet, "/api/activity?app=hidden-app", cookie)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for inaccessible app slug, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestListActivityAppSlugAllowedNonAdmin verifies non-admin CAN query an app they have access to.
func TestListActivityAppSlugAllowedNonAdmin(t *testing.T) {
	srv, st := newTestServer(t)
	appID := seedApp(t, st, "visible-app")
	uid := int64(1)
	seedAudit(t, st, store.AuditEntry{AppID: &appID, AppSlug: "visible-app", Category: "deploy", Action: "deploy_succeeded", Summary: "ok"})
	seedAudit(t, st, store.AuditEntry{ActorUserID: &uid, Category: "auth", Action: "login", Summary: "admin login"})

	cookie := makeUserCookie(t, srv, st, "visible-user", appID)
	w := doRequest(t, srv, http.MethodGet, "/api/activity?app=visible-app", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries := resp["entries"].([]any)
	if len(entries) == 0 {
		t.Error("expected at least one entry for accessible app")
	}
}

// TestRecentActivityLimit verifies recent endpoint default limit and max clamp.
func TestRecentActivityLimit(t *testing.T) {
	srv, st := newTestServer(t)
	uid := int64(1)

	// seed 20 entries
	for i := 0; i < 20; i++ {
		seedAudit(t, st, store.AuditEntry{
			ActorUserID: &uid,
			Category:    "auth",
			Action:      "login",
			Summary:     fmt.Sprintf("entry-%d", i),
			CreatedAt:   time.Now(),
		})
	}

	cookie := makeAdminCookie(t, srv)

	// default (no limit param) => 8
	w := doRequest(t, srv, http.MethodGet, "/api/activity/recent", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries := resp["entries"].([]any)
	if len(entries) != 8 {
		t.Errorf("default limit: expected 8 entries, got %d", len(entries))
	}

	// limit=100 exceeds max of 50 => should return 20 (all seeded)
	w2 := doRequest(t, srv, http.MethodGet, "/api/activity/recent?limit=100", cookie)
	json.NewDecoder(w2.Body).Decode(&resp)
	entries2 := resp["entries"].([]any)
	if len(entries2) != 20 {
		t.Errorf("clamped limit: expected 20 entries, got %d", len(entries2))
	}
}
