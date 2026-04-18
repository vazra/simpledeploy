package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
