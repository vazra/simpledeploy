package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

// setupUserTestServer creates a server with a super_admin user and returns the
// server, store, and a session cookie for the super_admin.
func setupUserTestServer(t *testing.T) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	srv, st := setupAuthTestServer(t)

	// Log in to get a session cookie.
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": "admin",
		"password": "password123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", w.Code)
	}
	var sessionCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}
	return srv, st, sessionCookie
}

// authedRequest builds a request with the given session cookie attached.
func authedRequest(t *testing.T, method, path string, body any, cookie *http.Cookie) *http.Request {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.AddCookie(cookie)
	return req
}

// loginAs creates a user, logs in, and returns their session cookie.
func loginAs(t *testing.T, srv *Server, st *store.Store, username, password, role string) *http.Cookie {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := st.CreateUser(username, hash, role, "", ""); err != nil {
		t.Fatalf("create user %q: %v", username, err)
	}
	w := postJSON(t, srv, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login %q status = %d, want 200", username, w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	t.Fatalf("no session cookie for %q", username)
	return nil
}

func TestCreateUser(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPost, "/api/users", map[string]string{
		"username": "newuser",
		"password": "pass1234",
		"role":     "admin",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["username"] != "newuser" {
		t.Errorf("username = %q, want newuser", resp["username"])
	}
	if resp["role"] != "admin" {
		t.Errorf("role = %q, want admin", resp["role"])
	}
	if _, ok := resp["password_hash"]; ok {
		t.Error("response must not include password_hash")
	}
}

func TestCreateUserForbidden(t *testing.T) {
	srv, st, _ := setupUserTestServer(t)

	// create an admin (non-super_admin) and log in as them
	adminCookie := loginAs(t, srv, st, "regularadmin", "pass123", "admin")

	req := authedRequest(t, http.MethodPost, "/api/users", map[string]string{
		"username": "anotheruser",
		"password": "pass1234",
		"role":     "admin",
	}, adminCookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestListUsers(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	// create a second user
	hash, _ := auth.HashPassword("p")
	if _, err := st.CreateUser("bob", hash, "admin", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	req := authedRequest(t, http.MethodGet, "/api/users", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var users []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&users); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(users) < 2 {
		t.Errorf("got %d users, want >= 2", len(users))
	}
	for _, u := range users {
		if _, ok := u["password_hash"]; ok {
			t.Error("list users must not include password_hash")
		}
	}
}

func TestDeleteUser(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	hash, _ := auth.HashPassword("p")
	created, err := st.CreateUser("todelete", hash, "admin", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	req := authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/users/%d", created.ID), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}

func TestGrantAppAccess(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	// create app
	app := &store.App{Name: "myapp", Slug: "myapp", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// create target user
	hash, _ := auth.HashPassword("p")
	target, err := st.CreateUser("targetuser", hash, "admin", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	req := authedRequest(t, http.MethodPost, fmt.Sprintf("/api/users/%d/access", target.ID),
		map[string]string{"app_slug": "myapp"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}

func TestCreateAPIKey(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPost, "/api/apikeys",
		map[string]string{"name": "claude-code"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	key, ok := resp["key"].(string)
	if !ok || !strings.HasPrefix(key, "sd_") {
		t.Errorf("key = %q, want string starting with sd_", resp["key"])
	}
	if resp["name"] != "claude-code" {
		t.Errorf("name = %q, want claude-code", resp["name"])
	}
}

func TestListAPIKeys(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	// create two keys
	for _, name := range []string{"key1", "key2"} {
		req := authedRequest(t, http.MethodPost, "/api/apikeys",
			map[string]string{"name": name}, cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %q status = %d", name, w.Code)
		}
	}

	req := authedRequest(t, http.MethodGet, "/api/apikeys", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var keys []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&keys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(keys) < 2 {
		t.Errorf("got %d keys, want >= 2", len(keys))
	}
}

func TestGetMe(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodGet, "/api/me", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/me status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["username"] != "admin" {
		t.Errorf("username = %v, want admin", resp["username"])
	}
	if resp["role"] != "super_admin" {
		t.Errorf("role = %v, want super_admin", resp["role"])
	}
}

func TestUpdateMe(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPut, "/api/me", map[string]string{
		"display_name": "Admin User",
		"email":        "admin@example.com",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/me status = %d, want 200", w.Code)
	}

	user, err := st.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.DisplayName != "Admin User" {
		t.Errorf("display_name = %q, want %q", user.DisplayName, "Admin User")
	}
	if user.Email != "admin@example.com" {
		t.Errorf("email = %q, want %q", user.Email, "admin@example.com")
	}
}

func TestChangePassword(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPut, "/api/me/password", map[string]string{
		"current_password": "password123",
		"new_password":     "newpass456",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/me/password status = %d, want 200", w.Code)
	}

	user, _ := st.GetUserByUsername("admin")
	if !auth.CheckPassword(user.PasswordHash, "newpass456") {
		t.Error("new password should be valid")
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPut, "/api/me/password", map[string]string{
		"current_password": "wrongpassword",
		"new_password":     "newpass456",
	}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	// create a key
	req := authedRequest(t, http.MethodPost, "/api/apikeys",
		map[string]string{"name": "torevoke"}, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create key status = %d", w.Code)
	}
	var created map[string]any
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	id := int64(created["id"].(float64))

	req = authedRequest(t, http.MethodDelete, fmt.Sprintf("/api/apikeys/%d", id), nil, cookie)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}
