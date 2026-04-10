package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestHandleUpdateDomain(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
    labels:
      simpledeploy.domain: old.example.com
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: composePath, Status: "running"}, nil)

	body, _ := json.Marshal(map[string]string{"domain": "new.example.com"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/myapp/domain", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(updated), "new.example.com") {
		t.Errorf("expected new.example.com in compose, got:\n%s", string(updated))
	}
	if strings.Contains(string(updated), "old.example.com") {
		t.Errorf("old.example.com still present in compose")
	}
}

func TestHandleUpdateDomain_NoExistingLabel(t *testing.T) {
	srv, s := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)

	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	composeContent := `services:
  web:
    image: nginx
`
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	s.UpsertApp(&store.App{Name: "noapp", Slug: "noapp", ComposePath: composePath, Status: "stopped"}, nil)

	body, _ := json.Marshal(map[string]string{"domain": "brand.new.com"})
	req := httptest.NewRequest(http.MethodPut, "/api/apps/noapp/domain", bytes.NewReader(body))
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(updated), "brand.new.com") {
		t.Errorf("expected brand.new.com in compose, got:\n%s", string(updated))
	}
	if !strings.Contains(string(updated), "simpledeploy.domain") {
		t.Errorf("expected simpledeploy.domain label in compose, got:\n%s", string(updated))
	}
}
