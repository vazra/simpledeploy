package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

func newTrustTestServer(t *testing.T, tlsMode, dataDir string) *Server {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	jwtMgr := auth.NewJWTManager("test-secret", 24*time.Hour)
	rl := auth.NewRateLimiter(100, time.Minute)
	srv := NewServer(0, db, jwtMgr, rl)
	srv.SetTLSMode(tlsMode)
	srv.SetDataDir(dataDir)
	return srv
}

func TestTrustPageLocalMode(t *testing.T) {
	srv := newTrustTestServer(t, "local", t.TempDir())
	req := httptest.NewRequest("GET", "/trust", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
}

func TestTrustPage404WhenNotLocal(t *testing.T) {
	srv := newTrustTestServer(t, "auto", t.TempDir())
	req := httptest.NewRequest("GET", "/trust", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCACertDownload(t *testing.T) {
	dataDir := t.TempDir()
	caDir := filepath.Join(dataDir, "caddy", "pki", "authorities", "local")
	os.MkdirAll(caDir, 0755)
	os.WriteFile(filepath.Join(caDir, "root.crt"), []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n"), 0644)
	srv := newTrustTestServer(t, "local", dataDir)
	req := httptest.NewRequest("GET", "/api/tls/ca.crt", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-pem-file" {
		t.Errorf("Content-Type = %q, want application/x-pem-file", ct)
	}
}

func TestCACert404WhenNotLocal(t *testing.T) {
	srv := newTrustTestServer(t, "auto", t.TempDir())
	req := httptest.NewRequest("GET", "/api/tls/ca.crt", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCACert404WhenFileNotExist(t *testing.T) {
	srv := newTrustTestServer(t, "local", t.TempDir())
	req := httptest.NewRequest("GET", "/api/tls/ca.crt", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
