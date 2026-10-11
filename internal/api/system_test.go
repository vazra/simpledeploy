package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemInfoDeploymentFields(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp systemInfoResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	sd := resp.SimpleDeploy
	if sd.DeploymentMode == "" {
		t.Error("deployment_mode is empty")
	}
	if sd.DeploymentLabel == "" {
		t.Error("deployment_label is empty")
	}
	if sd.DeploymentMode != "native" {
		t.Errorf("deployment_mode = %q, want %q", sd.DeploymentMode, "native")
	}
}

func TestPruneRejectsUnknownTier(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	for _, path := range []string{"/api/system/prune/metrics", "/api/system/prune/request-stats"} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, authedRequest(t, http.MethodPost, path, map[string]any{"tier": "1hr", "days": 1}, cookie))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400; body: %s", path, rr.Code, rr.Body.String())
		}
		if body := rr.Body.String(); !strings.Contains(body, `"1hr"`) || !strings.Contains(body, "raw, 1m, 5m, 1h, 1d") {
			t.Errorf("%s: body = %q, want the bad tier and the valid ones", path, body)
		}
	}
}

func TestPruneTierDefaultsAndValid(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	cases := []struct {
		body any
		tier string
	}{
		{nil, "raw"},
		{map[string]any{"days": 1}, "raw"},
		{map[string]any{"tier": "1h", "days": 1}, "1h"},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, authedRequest(t, http.MethodPost, "/api/system/prune/metrics", tc.body, cookie))
		if rr.Code != http.StatusOK {
			t.Fatalf("body %v: status = %d, want 200; body: %s", tc.body, rr.Code, rr.Body.String())
		}
		var resp pruneResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(resp.Message, "["+tc.tier+"]") {
			t.Errorf("body %v: message = %q, want tier %s", tc.body, resp.Message, tc.tier)
		}
	}
}

func TestPruneRejectsInvalidJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	req := httptest.NewRequest(http.MethodPost, "/api/system/prune/metrics", strings.NewReader(`{"tier": "raw",`))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}
