package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/gitsync"
	"github.com/vazra/simpledeploy/internal/store"
)

func TestHandleGitStatus_NilSyncer(t *testing.T) {
	srv, _ := newTestServer(t)
	// gs is nil by default

	req := httptest.NewRequest("GET", "/api/git/status", nil)
	w := httptest.NewRecorder()
	srv.handleGitStatus(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleGitWebhook_NilSyncer(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/git/webhook", nil)
	w := httptest.NewRecorder()
	srv.handleGitWebhook(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// TestGitWebhookRateLimit: after N requests the rate limiter returns 429.
func TestGitWebhookRateLimit(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	// Tight rate limiter: 3 requests per minute.
	rl := auth.NewRateLimiter(3, time.Minute)
	srv := NewServer(0, st, jwtMgr, rl)

	// Build a real Syncer (disabled so Start is a no-op but WebhookHandler works).
	// We use Enabled=true with a webhook secret so WebhookHandler returns non-nil.
	// The Syncer is configured to point at a bare dir but we won't call Start.
	secret := "test-webhook-secret"
	bareDir := t.TempDir()
	appsDir := t.TempDir()
	gs, err := gitsync.New(gitsync.Config{
		Enabled:          true,
		Remote:           "file://" + bareDir,
		Branch:           "main",
		AppsDir:          appsDir,
		WebhookSecret:    secret,
		PollInterval:     0,
		AutoPushEnabled:  true,
		AutoApplyEnabled: true,
		WebhookEnabled:   true,
	}, st, nil, nil)
	if err != nil {
		t.Fatalf("gitsync.New: %v", err)
	}
	srv.SetGitSync(gs)

	// Build a valid HMAC signature.
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Fire 5 requests via the full Handler (which applies rate-limit middleware).
	handler := srv.Handler()
	var got429 bool
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/api/git/webhook", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", sig)
		req.RemoteAddr = "10.0.0.1:9999" // fixed IP so limiter tracks correctly
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Error("expected a 429 after exceeding rate limit, none received")
	}
}

// TestApplyPending_NilSyncer: returns 503 when gs is nil.
func TestApplyPending_NilSyncer(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/git/apply-pending", nil)
	w := httptest.NewRecorder()
	srv.handleGitApplyPending(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// TestApplyPending_AuthRequired: unauthenticated request returns 401.
func TestApplyPending_AuthRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/git/apply-pending", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestApplyPending_SuperAdminOnly: non-super-admin returns 403.
func TestApplyPending_SuperAdminOnly(t *testing.T) {
	srv, st := newTestServer(t)
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	if _, err := st.CreateUser("regular", "hashed", "manage", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	tok, _ := jwtMgr.Generate(2, "regular", "admin")
	req := httptest.NewRequest("POST", "/api/git/apply-pending", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tok})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// TestGitStatusAfterStartFailure: when gs is nil (Start errored and caller set
// gs=nil), /api/git/status returns 503. This matches the current behavior in
// handleGitStatus and the UI which handles 503 gracefully.
func TestGitStatusAfterStartFailure(t *testing.T) {
	srv, _ := newTestServer(t)
	// gs remains nil — simulates Start returning an error and caller not setting gs.
	// Current behavior: 503 Service Unavailable.
	req := httptest.NewRequest("GET", "/api/git/status", nil)
	w := httptest.NewRecorder()
	srv.handleGitStatus(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 when gs=nil, got %d", w.Code)
	}

	// Also test with a Syncer that has Enabled=false (disabled via config).
	// In that case the server could inject a non-nil syncer with Enabled=false.
	// Status() still returns useful info (Enabled: false, Remote, Branch).
	gs, err := gitsync.New(gitsync.Config{
		Enabled:          false,
		Remote:           "file:///unused",
		Branch:           "main",
		AutoPushEnabled:  true,
		AutoApplyEnabled: true,
		WebhookEnabled:   true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("gitsync.New disabled: %v", err)
	}
	srv.SetGitSync(gs)

	req2 := httptest.NewRequest("GET", "/api/git/status", nil)
	w2 := httptest.NewRecorder()
	srv.handleGitStatus(w2, req2)

	// When gs is non-nil (even disabled), the handler returns 200 with status JSON.
	if w2.Code != http.StatusOK {
		t.Errorf("want 200 with disabled syncer, got %d", w2.Code)
	}
}
