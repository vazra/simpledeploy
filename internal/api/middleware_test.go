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

	token, err := srv.jwt.Generate(1, "manage", "super_admin", 1)
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

	user, err := s.CreateUser("bob", "hash", "manage", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	plaintext, hash, err := auth.GenerateAPIKey("test-master-secret")
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	if _, err := s.CreateAPIKey(user.ID, hash, "test-key", nil); err != nil {
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
	if captured.Username != "bob" || captured.Role != "manage" {
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

	user, err := s.CreateUser("carol", "hash", "manage", "", "")
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
	req = setAuthUser(req, &AuthUser{ID: user.ID, Username: "carol", Role: "manage"})

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

		token, err := srv.jwt.Generate(1, "manage", "super_admin", 1)
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

		user, err := s.CreateUser("eve", "hash", "manage", "", "")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		plaintext, hash, err := auth.GenerateAPIKey("test-master-secret")
		if err != nil {
			t.Fatalf("generate api key: %v", err)
		}
		if _, err := s.CreateAPIKey(user.ID, hash, "test-key", nil); err != nil {
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

func TestMutatingAppMiddleware(t *testing.T) {
	srv, s := newMiddlewareTestServer(t)

	if err := s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	app, _ := s.GetAppBySlug("myapp")
	manageGranted, _ := s.CreateUser("mgr1", "h", "manage", "", "")
	if err := s.GrantAppAccess(manageGranted.ID, app.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	manageNoGrant, _ := s.CreateUser("mgr2", "h", "manage", "", "")
	viewer, _ := s.CreateUser("vw1", "h", "viewer", "", "")
	_ = s.GrantAppAccess(viewer.ID, app.ID)

	mux := http.NewServeMux()
	mux.Handle("POST /api/apps/{slug}/restart", srv.mutatingAppMiddleware(okHandler))

	cases := []struct {
		name string
		user *AuthUser
		want int
	}{
		{"super_admin always passes", &AuthUser{ID: 999, Username: "root", Role: "super_admin"}, 200},
		{"manage with grant passes", &AuthUser{ID: manageGranted.ID, Username: "mgr1", Role: "manage"}, 200},
		{"manage without grant 404", &AuthUser{ID: manageNoGrant.ID, Username: "mgr2", Role: "manage"}, 404},
		{"viewer forbidden 403", &AuthUser{ID: viewer.ID, Username: "vw1", Role: "viewer"}, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/restart", nil)
			req = setAuthUser(req, tc.user)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestAppAccessUnauthorizedUser(t *testing.T) {
	srv, s := newMiddlewareTestServer(t)

	user, err := s.CreateUser("dave", "hash", "manage", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Create app but do NOT grant access
	if err := s.UpsertApp(&store.App{Name: "secret", Slug: "secret", ComposePath: "/tmp/1.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/apps/secret", nil)
	req = setAuthUser(req, &AuthUser{ID: user.ID, Username: "dave", Role: "manage"})

	mux := http.NewServeMux()
	mux.Handle("GET /api/apps/{slug}", srv.appAccessMiddleware(okHandler))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCanMutateForApp(t *testing.T) {
	srv, s := newMiddlewareTestServer(t)
	if err := s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	app, _ := s.GetAppBySlug("myapp")
	mgrGranted, _ := s.CreateUser("mg1", "h", "manage", "", "")
	if err := s.GrantAppAccess(mgrGranted.ID, app.ID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	mgrNoGrant, _ := s.CreateUser("mg2", "h", "manage", "", "")
	viewer, _ := s.CreateUser("vw", "h", "viewer", "", "")
	_ = s.GrantAppAccess(viewer.ID, app.ID)

	cases := []struct {
		name  string
		user  *AuthUser
		appID *int64
		want  int
	}{
		{"super_admin nil app passes", &AuthUser{ID: 999, Role: "super_admin"}, nil, 200},
		{"super_admin with app passes", &AuthUser{ID: 999, Role: "super_admin"}, &app.ID, 200},
		{"manage nil app forbidden", &AuthUser{ID: mgrGranted.ID, Role: "manage"}, nil, 403},
		{"manage granted app passes", &AuthUser{ID: mgrGranted.ID, Role: "manage"}, &app.ID, 200},
		{"manage no grant 404", &AuthUser{ID: mgrNoGrant.ID, Role: "manage"}, &app.ID, 404},
		{"viewer always 403", &AuthUser{ID: viewer.ID, Role: "viewer"}, &app.ID, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			req = setAuthUser(req, tc.user)
			w := httptest.NewRecorder()
			ok := srv.canMutateForApp(w, req, tc.appID)
			if ok && tc.want != 200 {
				t.Errorf("expected reject (status %d) but ok=true", tc.want)
			}
			if !ok && tc.want == 200 {
				t.Errorf("expected ok but rejected with %d", w.Code)
			}
			if !ok && w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
