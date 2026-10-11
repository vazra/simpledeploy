package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

// Alert rule app scope by slug: the UI sends app_slug, not app_id.

// seedAlertApps creates apps "alpha" and "beta" and returns their ids.
func seedAlertApps(t *testing.T, st *store.Store) (alphaID, betaID int64) {
	t.Helper()
	for _, slug := range []string{"alpha", "beta"} {
		if err := st.UpsertApp(&store.App{Name: slug, Slug: slug, ComposePath: "/tmp/" + slug + ".yml", Status: "running"}, nil); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}
	a, _ := st.GetAppBySlug("alpha")
	b, _ := st.GetAppBySlug("beta")
	return a.ID, b.ID
}

func doRuleRequest(t *testing.T, srv *Server, cookie *http.Cookie, method, path string, body map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, authedRequest(t, method, path, body, cookie))
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func ruleBody(whID int64, threshold float64, scope map[string]any) map[string]any {
	b := map[string]any{
		"metric": "cpu_pct", "operator": ">", "threshold": threshold,
		"duration_sec": 60, "webhook_id": whID, "enabled": true,
	}
	for k, v := range scope {
		b[k] = v
	}
	return b
}

func storedRule(t *testing.T, st *store.Store, id int64) store.AlertRule {
	t.Helper()
	rules, err := st.ListAlertRules(nil)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	for _, r := range rules {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("rule %d not stored", id)
	return store.AlertRule{}
}

func mustUserID(t *testing.T, st *store.Store, username string) int64 {
	t.Helper()
	u, err := st.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("get user %s: %v", username, err)
	}
	return u.ID
}

func TestAlertRuleAppSlugKeepsApp(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)
	alphaID, _ := seedAlertApps(t, st)
	whID := int64(createTestWebhook(t, srv, cookie, "slugwh")["id"].(float64))

	w, resp := doRuleRequest(t, srv, cookie, http.MethodPost, "/api/alerts/rules", ruleBody(whID, 80, map[string]any{"app_slug": "alpha"}))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if resp["app_slug"] != "alpha" {
		t.Errorf("create response app_slug = %v, want alpha", resp["app_slug"])
	}
	ruleID := int64(resp["id"].(float64))
	if got := storedRule(t, st, ruleID); got.AppID == nil || *got.AppID != alphaID {
		t.Fatalf("created rule app_id = %v, want %d (rule became all-apps)", got.AppID, alphaID)
	}

	// Editing in the UI sends app_slug again; the app must stay.
	path := fmt.Sprintf("/api/alerts/rules/%d", ruleID)
	w, resp = doRuleRequest(t, srv, cookie, http.MethodPut, path, ruleBody(whID, 90, map[string]any{"app_slug": "alpha"}))
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if resp["app_slug"] != "alpha" {
		t.Errorf("update response app_slug = %v, want alpha", resp["app_slug"])
	}
	got := storedRule(t, st, ruleID)
	if got.AppID == nil || *got.AppID != alphaID {
		t.Fatalf("updated rule app_id = %v, want %d (edit turned rule into all-apps)", got.AppID, alphaID)
	}
	if got.Threshold != 90 {
		t.Errorf("threshold = %v, want 90", got.Threshold)
	}

	// app_id plus a matching app_slug (a GET round trip) is accepted.
	w, _ = doRuleRequest(t, srv, cookie, http.MethodPut, path, ruleBody(whID, 70, map[string]any{"app_id": alphaID, "app_slug": "alpha"}))
	if w.Code != http.StatusOK {
		t.Fatalf("update with matching app_id+app_slug status = %d; body: %s", w.Code, w.Body.String())
	}
	if got := storedRule(t, st, ruleID); got.AppID == nil || *got.AppID != alphaID {
		t.Fatalf("rule app_id = %v, want %d", got.AppID, alphaID)
	}

	// Empty app_slug (the "All Apps" option) makes it global for super_admin.
	w, _ = doRuleRequest(t, srv, cookie, http.MethodPut, path, ruleBody(whID, 70, map[string]any{"app_slug": ""}))
	if w.Code != http.StatusOK {
		t.Fatalf("update to all apps status = %d; body: %s", w.Code, w.Body.String())
	}
	if got := storedRule(t, st, ruleID); got.AppID != nil {
		t.Fatalf("rule app_id = %v, want nil (all apps)", *got.AppID)
	}
}

func TestAlertRuleAppSlugValidation(t *testing.T) {
	srv, st, cookie := setupAlertTestServer(t)
	alphaID, _ := seedAlertApps(t, st)
	whID := int64(createTestWebhook(t, srv, cookie, "valslugwh")["id"].(float64))
	ruleID := int64(createTestRule(t, srv, cookie, whID)["id"].(float64))
	path := fmt.Sprintf("/api/alerts/rules/%d", ruleID)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		scope  map[string]any
	}{
		{"create unknown slug", http.MethodPost, "/api/alerts/rules", map[string]any{"app_slug": "nope"}},
		{"update unknown slug", http.MethodPut, path, map[string]any{"app_slug": "nope"}},
		{"create mismatched id and slug", http.MethodPost, "/api/alerts/rules", map[string]any{"app_id": alphaID, "app_slug": "beta"}},
		{"update mismatched id and slug", http.MethodPut, path, map[string]any{"app_id": alphaID, "app_slug": "beta"}},
	} {
		w, _ := doRuleRequest(t, srv, cookie, tc.method, tc.path, ruleBody(whID, 50, tc.scope))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; body: %s", tc.name, w.Code, w.Body.String())
		}
	}
	if got := storedRule(t, st, ruleID); got.AppID != nil || got.Threshold != 80 {
		t.Errorf("rejected updates changed the rule: %+v", got)
	}
	if rules, _ := st.ListAlertRules(nil); len(rules) != 1 {
		t.Errorf("rejected creates stored rules: have %d rules, want 1", len(rules))
	}
}

func TestAlertRuleAppSlugManageUserScope(t *testing.T) {
	srv, st, adminCookie := setupUserTestServer(t)
	alphaID, betaID := seedAlertApps(t, st)
	whID := int64(createTestWebhook(t, srv, adminCookie, "mgrwh")["id"].(float64))

	mgrCookie := loginAs(t, srv, st, "mgr", "managepass1", "manage")
	if err := st.GrantAppAccess(mustUserID(t, st, "mgr"), alphaID); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// Own app by slug: allowed and kept.
	w, resp := doRuleRequest(t, srv, mgrCookie, http.MethodPost, "/api/alerts/rules", ruleBody(whID, 80, map[string]any{"app_slug": "alpha"}))
	if w.Code != http.StatusCreated {
		t.Fatalf("manage create for own app status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	ownID := int64(resp["id"].(float64))
	if got := storedRule(t, st, ownID); got.AppID == nil || *got.AppID != alphaID {
		t.Fatalf("manage rule app_id = %v, want %d", got.AppID, alphaID)
	}

	// Another app, or an app that does not exist: same 404, nothing stored.
	for _, slug := range []string{"beta", "nope"} {
		w, _ = doRuleRequest(t, srv, mgrCookie, http.MethodPost, "/api/alerts/rules", ruleBody(whID, 80, map[string]any{"app_slug": slug}))
		if w.Code != http.StatusNotFound {
			t.Errorf("manage create for %q status = %d, want 404; body: %s", slug, w.Code, w.Body.String())
		}
	}
	if rules, _ := st.ListAlertRules(nil); len(rules) != 1 {
		t.Fatalf("manage stored %d rules, want 1", len(rules))
	}

	// Moving their own rule to another app is refused and leaves it alone.
	ownPath := fmt.Sprintf("/api/alerts/rules/%d", ownID)
	w, _ = doRuleRequest(t, srv, mgrCookie, http.MethodPut, ownPath, ruleBody(whID, 99, map[string]any{"app_slug": "beta"}))
	if w.Code != http.StatusNotFound {
		t.Errorf("manage move to beta status = %d, want 404", w.Code)
	}
	// Mismatched id and slug, where the slug is an app they lack: 404, not
	// 400, so the answer does not reveal that the app exists.
	w, _ = doRuleRequest(t, srv, mgrCookie, http.MethodPut, ownPath, ruleBody(whID, 99, map[string]any{"app_id": alphaID, "app_slug": "beta"}))
	if w.Code != http.StatusNotFound {
		t.Errorf("manage mismatched id/slug status = %d, want 404", w.Code)
	}
	// Global rules stay super_admin only.
	w, _ = doRuleRequest(t, srv, mgrCookie, http.MethodPut, ownPath, ruleBody(whID, 99, map[string]any{"app_slug": ""}))
	if w.Code != http.StatusForbidden {
		t.Errorf("manage make global status = %d, want 403", w.Code)
	}
	if got := storedRule(t, st, ownID); got.AppID == nil || *got.AppID != alphaID || got.Threshold != 80 {
		t.Errorf("refused updates changed the rule: %+v", got)
	}

	// A rule on an app they lack cannot be pulled into their app.
	w, resp = doRuleRequest(t, srv, adminCookie, http.MethodPost, "/api/alerts/rules", ruleBody(whID, 80, map[string]any{"app_slug": "beta"}))
	if w.Code != http.StatusCreated {
		t.Fatalf("admin create beta rule status = %d; body: %s", w.Code, w.Body.String())
	}
	betaRuleID := int64(resp["id"].(float64))
	w, _ = doRuleRequest(t, srv, mgrCookie, http.MethodPut, fmt.Sprintf("/api/alerts/rules/%d", betaRuleID), ruleBody(whID, 1, map[string]any{"app_slug": "alpha"}))
	if w.Code != http.StatusNotFound {
		t.Errorf("manage edit of beta rule status = %d, want 404", w.Code)
	}
	if got := storedRule(t, st, betaRuleID); got.AppID == nil || *got.AppID != betaID || got.Threshold != 80 {
		t.Errorf("beta rule changed: %+v", got)
	}

	// Viewers cannot create rules, for their own app or a made-up one.
	viewerCookie := loginAs(t, srv, st, "v1", "viewerpass1", "viewer")
	if err := st.GrantAppAccess(mustUserID(t, st, "v1"), alphaID); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}
	for _, slug := range []string{"alpha", "nope"} {
		w, _ = doRuleRequest(t, srv, viewerCookie, http.MethodPost, "/api/alerts/rules", ruleBody(whID, 80, map[string]any{"app_slug": slug}))
		if w.Code != http.StatusForbidden {
			t.Errorf("viewer create for %q status = %d, want 403", slug, w.Code)
		}
	}
}
