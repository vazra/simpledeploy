package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

// TestListApps_FiltersForManageUser verifies non-super_admin callers only see
// apps they have access to.
func TestListApps_FiltersForManageUser(t *testing.T) {
	srv, st, adminCookie := setupUserTestServer(t)

	// Seed two apps.
	if err := st.UpsertApp(&store.App{Name: "alpha", Slug: "alpha", ComposePath: "/tmp/a.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if err := st.UpsertApp(&store.App{Name: "beta", Slug: "beta", ComposePath: "/tmp/b.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed beta: %v", err)
	}
	app1, err := st.GetAppBySlug("alpha")
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}

	// Manage user with access only to alpha.
	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	mgr, err := st.GetUserByUsername("mgr")
	if err != nil {
		t.Fatalf("get mgr: %v", err)
	}
	if err := st.GrantAppAccess(mgr.ID, app1.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// Manage user: only alpha.
	req := authedRequest(t, http.MethodGet, "/api/apps", nil, manageCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var apps []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("manage saw %d apps, want 1: %+v", len(apps), apps)
	}
	if apps[0]["Slug"] != "alpha" {
		t.Errorf("manage slug = %v, want alpha", apps[0]["Slug"])
	}

	// Super_admin: both apps.
	req = authedRequest(t, http.MethodGet, "/api/apps", nil, adminCookie)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status = %d", w.Code)
	}
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 2 {
		t.Errorf("admin saw %d apps, want 2", len(apps))
	}
}

// TestListApps_ViewerWithoutGrantsSeesNothing verifies a viewer with no grants
// gets an empty list (not an error).
func TestListApps_ViewerWithoutGrantsSeesNothing(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)
	if err := st.UpsertApp(&store.App{Name: "alpha", Slug: "alpha", ComposePath: "/tmp/a.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	viewerCookie := loginAs(t, srv, st, "viewer1", "viewerpass1", "viewer")
	req := authedRequest(t, http.MethodGet, "/api/apps", nil, viewerCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var apps []map[string]any
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 0 {
		t.Errorf("viewer saw %d apps, want 0", len(apps))
	}
}

// TestListUserAccess verifies the GET /api/users/{id}/access endpoint.
func TestListUserAccess(t *testing.T) {
	srv, st, adminCookie := setupUserTestServer(t)

	if err := st.UpsertApp(&store.App{Name: "alpha", Slug: "alpha", ComposePath: "/tmp/a.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := st.UpsertApp(&store.App{Name: "beta", Slug: "beta", ComposePath: "/tmp/b.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("seed beta: %v", err)
	}
	app1, err := st.GetAppBySlug("alpha")
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}

	// Create manage user and grant alpha.
	manageCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	mgr, _ := st.GetUserByUsername("mgr")
	if err := st.GrantAppAccess(mgr.ID, app1.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// Super_admin can list.
	req := authedRequest(t, http.MethodGet, "/api/users/"+strconv.FormatInt(mgr.ID, 10)+"/access", nil, adminCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var slugs []string
	if err := json.NewDecoder(w.Body).Decode(&slugs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(slugs) != 1 || slugs[0] != "alpha" {
		t.Errorf("slugs = %v, want [alpha]", slugs)
	}

	// Non-super_admin forbidden.
	req = authedRequest(t, http.MethodGet, "/api/users/"+strconv.FormatInt(mgr.ID, 10)+"/access", nil, manageCookie)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("manage caller status = %d, want 403", w.Code)
	}

	// Unknown user => 404.
	req = authedRequest(t, http.MethodGet, "/api/users/99999/access", nil, adminCookie)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown user status = %d, want 404", w.Code)
	}
}

