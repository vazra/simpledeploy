package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

func setupAuthTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = s.CreateUser("admin", hash, "super_admin", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	jwtMgr := auth.NewJWTManager("test-secret", 24*time.Hour)
	rl := auth.NewRateLimiter(10, time.Minute)
	srv := NewServer(0, s, jwtMgr, rl)
	return srv, s
}

func postJSON(t *testing.T, srv *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestLoginSuccess(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": "admin",
		"password": "password123",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// verify cookie set
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set")
	}
	if sessionCookie.Value == "" {
		t.Error("session cookie value is empty")
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["username"] != "admin" {
		t.Errorf("username = %q, want admin", resp["username"])
	}
	if resp["role"] != "super_admin" {
		t.Errorf("role = %q, want super_admin", resp["role"])
	}
}

func TestLoginWrongPassword(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": "nobody",
		"password": "password123",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestLoginRateLimited(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	jwtMgr := auth.NewJWTManager("test-secret", 24*time.Hour)
	// limit of 2 requests per minute
	rl := auth.NewRateLimiter(2, time.Minute)
	srv := NewServer(0, s, jwtMgr, rl)

	body := map[string]string{"username": "x", "password": "y"}
	var last *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		last = postJSON(t, srv, "/api/auth/login", body)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", last.Code)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie in response")
	}
	if sessionCookie.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want < 0", sessionCookie.MaxAge)
	}
}

func TestSetupFirstUser(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	jwtMgr := auth.NewJWTManager("test-secret", 24*time.Hour)
	srv := NewServer(0, s, jwtMgr, nil)

	w := postJSON(t, srv, "/api/setup", map[string]string{
		"username": "admin",
		"password": "securepassword",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["username"] != "admin" {
		t.Errorf("username = %q, want admin", resp["username"])
	}
	if resp["role"] != "super_admin" {
		t.Errorf("role = %q, want super_admin", resp["role"])
	}
}

func TestSetupBlockedWhenUsersExist(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	w := postJSON(t, srv, "/api/setup", map[string]string{
		"username": "admin2",
		"password": "password",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}
