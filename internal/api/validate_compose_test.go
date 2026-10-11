package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// validate-compose must refuse what deploy refuses, with the same messages,
// so the wizard never says "Valid compose file" for a deploy that fails.
func TestValidateComposeReportsDeployViolations(t *testing.T) {
	cases := map[string]string{
		"privileged": "services:\n  web:\n    image: nginx\n    privileged: true\n",
		"duplicate endpoint": "services:\n  web:\n    image: nginx\n    labels:\n" +
			"      simpledeploy.endpoints.0.domain: \"a.example.com\"\n" +
			"      simpledeploy.endpoints.0.port: \"80\"\n" +
			"      simpledeploy.endpoints.1.domain: \"a.example.com\"\n" +
			"      simpledeploy.endpoints.1.port: \"81\"\n",
		"other app folder": "services:\n  web:\n    image: nginx\n    volumes:\n      - ../other/data:/data\n",
		"include":          "include:\n  - other.yml\nservices:\n  web:\n    image: nginx\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			srv, appsDir := newDeployTestServer(t)
			protectApps(t, appsDir)
			encoded := base64.StdEncoding.EncodeToString([]byte(content))

			dw := doJSON(t, srv, http.MethodPost, "/api/apps/deploy", map[string]string{"name": "probe", "compose": encoded})
			if dw.Code != http.StatusBadRequest {
				t.Fatalf("deploy status = %d, want 400; body: %s", dw.Code, dw.Body.String())
			}
			var deployResp struct {
				Violations []string `json:"violations"`
			}
			json.NewDecoder(dw.Body).Decode(&deployResp)
			if len(deployResp.Violations) == 0 {
				t.Fatalf("deploy gave no violations: %s", dw.Body.String())
			}

			vw := doJSON(t, srv, http.MethodPost, "/api/apps/validate-compose", map[string]string{"compose": encoded})
			if vw.Code != http.StatusOK {
				t.Fatalf("validate status = %d, want 200; body: %s", vw.Code, vw.Body.String())
			}
			var resp struct {
				Valid  bool     `json:"valid"`
				Errors []string `json:"errors"`
			}
			json.NewDecoder(vw.Body).Decode(&resp)
			if resp.Valid {
				t.Fatalf("validate said valid; deploy refused with %v", deployResp.Violations)
			}
			if !reflect.DeepEqual(resp.Errors, deployResp.Violations) {
				t.Errorf("validate errors = %q, want deploy violations %q", resp.Errors, deployResp.Violations)
			}
		})
	}
}

// Files the deploy accepts must still validate, including relative binds
// inside the app's own folder.
func TestValidateComposeAcceptsAppFolderBind(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	protectApps(t, appsDir)
	encoded := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx\n    ports:\n      - \"8080:80\"\n    volumes:\n      - ./data:/data\n"))
	w := doJSON(t, srv, http.MethodPost, "/api/apps/validate-compose", map[string]string{"compose": encoded})
	var resp struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Valid {
		t.Fatalf("valid = false, errors = %v", resp.Errors)
	}
}
