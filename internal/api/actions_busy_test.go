package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/deployer"
)

// busyReconciler has a deploy in flight for every app.
type busyReconciler struct {
	mockReconcilerFull
	rollbackErr error
}

func (b *busyReconciler) IsDeploying(string) bool { return true }
func (b *busyReconciler) RollbackOne(ctx context.Context, slug string, id int64) error {
	b.calls = append(b.calls, fmt.Sprintf("RollbackOne:%s:%d", slug, id))
	return b.rollbackErr
}

func postAction(t *testing.T, srv *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestRollbackWhileDeployingReturns409(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetAppsDir(t.TempDir())
	srv.SetReconciler(&busyReconciler{rollbackErr: deployer.ErrDeployInProgress})
	verID := seedVersion(t, srv, "myapp")

	w := postAction(t, srv, "/api/apps/myapp/rollback", map[string]any{"version_id": verID})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "a deploy is already running for this app") {
		t.Errorf("body = %q, want the busy message", w.Body.String())
	}
}

func TestRollbackComposeFailureSaysWhy(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetAppsDir(t.TempDir())
	ce := &deployer.ComposeError{Op: "compose up", Detail: "Bind for 127.0.0.1:8080 failed: port is already allocated", Err: errors.New("exit status 1")}
	rec := &busyReconciler{rollbackErr: fmt.Errorf("redeploy: %w", ce)}
	srv.SetReconciler(&notBusy{rec})
	verID := seedVersion(t, srv, "myapp")

	w := postAction(t, srv, "/api/apps/myapp/rollback", map[string]any{"version_id": verID})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "port is already allocated") {
		t.Errorf("body = %q, want the compose error", w.Body.String())
	}
}

type notBusy struct{ *busyReconciler }

func (notBusy) IsDeploying(string) bool { return false }

func TestRestartAndPullWhileDeployingReturn409(t *testing.T) {
	for _, action := range []string{"restart", "pull"} {
		srv, _ := newTestServer(t)
		srv.SetAppsDir(t.TempDir())
		rec := &busyReconciler{}
		srv.SetReconciler(rec)

		w := postAction(t, srv, "/api/apps/myapp/"+action, nil)
		if w.Code != http.StatusConflict {
			t.Fatalf("%s: status = %d, want 409; body: %s", action, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "a deploy is already running for this app") {
			t.Errorf("%s: body = %q, want the busy message", action, w.Body.String())
		}
		if len(rec.calls) != 0 {
			t.Errorf("%s: calls = %v, want none", action, rec.calls)
		}
	}
}
