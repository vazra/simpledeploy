package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestHandleGetEnv(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	appDir := t.TempDir()
	composePath := filepath.Join(appDir, "docker-compose.yml")
	envPath := filepath.Join(appDir, ".env")

	if err := os.WriteFile(envPath, []byte("FOO=bar\nBAZ=qux\n# comment\n\nEMPTY=\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	s.UpsertApp(&store.App{Name: "envapp", Slug: "envapp", ComposePath: composePath, Status: "running"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/envapp/env", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var vars []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&vars); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(vars) != 3 {
		t.Fatalf("got %d vars, want 3", len(vars))
	}

	found := map[string]string{}
	for _, v := range vars {
		found[v["key"]] = v["value"]
	}
	if found["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", found["FOO"])
	}
	if found["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want qux", found["BAZ"])
	}
	if found["EMPTY"] != "" {
		t.Errorf("EMPTY = %q, want empty string", found["EMPTY"])
	}
}

func TestHandleGetEnv_NoFile(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	appDir := t.TempDir()
	composePath := filepath.Join(appDir, "docker-compose.yml")

	s.UpsertApp(&store.App{Name: "noenvapp", Slug: "noenvapp", ComposePath: composePath, Status: "running"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/noenvapp/env", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var vars []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&vars); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(vars) != 0 {
		t.Errorf("got %d vars, want 0", len(vars))
	}
}

func TestHandlePutEnv(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	appDir := t.TempDir()
	composePath := filepath.Join(appDir, "docker-compose.yml")

	s.UpsertApp(&store.App{Name: "putenvapp", Slug: "putenvapp", ComposePath: composePath, Status: "running"}, nil)

	vars := []map[string]string{
		{"key": "DB_HOST", "value": "localhost"},
		{"key": "DB_PORT", "value": "5432"},
	}
	body, _ := json.Marshal(vars)

	req := httptest.NewRequest(http.MethodPut, "/api/apps/putenvapp/env", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}

	// verify file written
	envPath := filepath.Join(appDir, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	data := string(content)
	if !bytes.Contains(content, []byte("DB_HOST=localhost\n")) {
		t.Errorf(".env missing DB_HOST=localhost, got:\n%s", data)
	}
	if !bytes.Contains(content, []byte("DB_PORT=5432\n")) {
		t.Errorf(".env missing DB_PORT=5432, got:\n%s", data)
	}
}
