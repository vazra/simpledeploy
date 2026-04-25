package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

// newMiddlewareTestServer creates a server with a real JWT manager and store.
func newMiddlewareTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	// Create test users so auth middleware can validate JWT user existence
	if _, err := s.CreateUser("admin", "hashed", "super_admin", "", ""); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	jwtMgr := auth.NewJWTManager("middleware-test-secret", time.Hour)
	srv := NewServer(0, s, jwtMgr, nil)
	return srv, s
}

// okHandler is a simple 200 handler used to verify middleware passes through.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestAuthMiddlewareNoAuth(t *testing.T) {
	srv, _ := newMiddlewareTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.authMiddleware(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddlewareValidJWT(t *testing.T) {
	srv, _ := newMiddlewareTestServer(t)

	token, err := srv.jwt.Generate(1, "admin", "super_admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	var captured *AuthUser
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetAuthUser(r)
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	srv.authMiddleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if captured == nil {
		t.Fatal("auth user not set in context")
	}
	if captured.ID != 1 || captured.Username != "admin" || captured.Role != "super_admin" {
		t.Errorf("auth user = %+v, want {1 admin super_admin}", captured)
	}
}

func TestAuthMiddlewareValidAPIKey(t *testing.T) {
	srv, s := newMiddlewareTestServer(t)
	srv.SetMasterSecret("test-master-secret")

	user, err := s.CreateUser("bob", "hash", "admin", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	plaintext, hash, err := auth.GenerateAPIKey("test-master-secret")
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	if _, err := s.CreateAPIKey(user.ID, hash, "test-key"); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)

	var captured *AuthUser
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetAuthUser(r)
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	srv.authMiddleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if captured == nil {
		t.Fatal("auth user not set in context")
	}
	if captured.Username != "bob" || captured.Role != "admin" {
		t.Errorf("auth user = %+v, want {bob admin}", captured)
	}
}

func TestAuthMiddlewareInvalidJWT(t *testing.T) {
	srv, _ := newMiddlewareTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "not-a-valid-jwt"})

	w := httptest.NewRecorder()
	srv.authMiddleware(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAppAccessSuperAdmin(t *testing.T) {
	srv, _ := newMiddlewareTestServer(t)

	// super_admin should bypass app access check even for unknown slugs
	req := httptest.NewRequest(http.MethodGet, "/api/apps/anything", nil)
	req = setAuthUser(req, &AuthUser{ID: 1, Username: "root", Role: "super_admin"})

	w := httptest.NewRecorder()
	srv.appAccessMiddleware(okHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAppAccessAuthorizedUser(t *testing.T) {
	srv, s := newMiddlewareTestServer(t)

	user, err := s.CreateUser("carol", "hash", "admin", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	app, err := s.GetAppBySlug("myapp")
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	if err := s.GrantAppAccess(user.ID, app.ID); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	// Simulate a request that has {slug} set via PathValue - use the full route
	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp", nil)
	req = setAuthUser(req, &AuthUser{ID: user.ID, Username: "carol", Role: "admin"})

	// We need PathValue to work, so register through the mux
	mux := http.NewServeMux()
	mux.Handle("GET /api/apps/{slug}", srv.appAccessMiddleware(okHandler))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuditCtxFromAuth(t *testing.T) {
	t.Run("cookie/JWT sets source=ui", func(t *testing.T) {
		srv, _ := newMiddlewareTestServer(t)

		token, err := srv.jwt.Generate(1, "admin", "super_admin")
		if err != nil {
			t.Fatalf("generate token: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: token})

		var captured audit.Ctx
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = audit.From(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		srv.authMiddleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if captured.ActorName != "admin" {
			t.Errorf("ActorName = %q, want %q", captured.ActorName, "admin")
		}
		if captured.ActorSource != "ui" {
			t.Errorf("ActorSource = %q, want %q", captured.ActorSource, "ui")
		}
		if captured.ActorUserID == nil || *captured.ActorUserID != 1 {
			t.Errorf("ActorUserID = %v, want 1", captured.ActorUserID)
		}
		if captured.IP == "" {
			t.Error("IP is empty, want non-empty")
		}
	})

	t.Run("API key sets source=api", func(t *testing.T) {
		srv, s := newMiddlewareTestServer(t)
		srv.SetMasterSecret("test-master-secret")

		user, err := s.CreateUser("eve", "hash", "admin", "", "")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		plaintext, hash, err := auth.GenerateAPIKey("test-master-secret")
		if err != nil {
			t.Fatalf("generate api key: %v", err)
		}
		if _, err := s.CreateAPIKey(user.ID, hash, "test-key"); err != nil {
			t.Fatalf("create api key: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)

		var captured audit.Ctx
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = audit.From(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		srv.authMiddleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if captured.ActorName != "eve" {
			t.Errorf("ActorName = %q, want %q", captured.ActorName, "eve")
		}
		if captured.ActorSource != "api" {
			t.Errorf("ActorSource = %q, want %q", captured.ActorSource, "api")
		}
		if captured.ActorUserID == nil || *captured.ActorUserID != user.ID {
			t.Errorf("ActorUserID = %v, want %d", captured.ActorUserID, user.ID)
		}
		if captured.IP == "" {
			t.Error("IP is empty, want non-empty")
		}
	})
}

func TestAppAccessUnauthorizedUser(t *testing.T) {
	srv, s := newMiddlewareTestServer(t)

	user, err := s.CreateUser("dave", "hash", "admin", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Create app but do NOT grant access
	if err := s.UpsertApp(&store.App{Name: "secret", Slug: "secret", ComposePath: "/tmp/1.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/apps/secret", nil)
	req = setAuthUser(req, &AuthUser{ID: user.ID, Username: "dave", Role: "admin"})

	mux := http.NewServeMux()
	mux.Handle("GET /api/apps/{slug}", srv.appAccessMiddleware(okHandler))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
