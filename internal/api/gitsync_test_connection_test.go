package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/vazra/simpledeploy/internal/auth"
)

// makeBareRepoWithMain creates a bare repo with one commit on refs/heads/main
// and returns its path.
func makeBareRepoWithMain(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	if _, err := git.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, _ := git.PlainInit(work, false)
	wt, _ := repo.Worktree()
	os.WriteFile(filepath.Join(work, "README"), []byte("hi"), 0o644)
	wt.Add("README")
	_, _ = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "x", Email: "x@x", When: time.Now()}})
	headRef, _ := repo.Head()
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), headRef.Hash())
	repo.Storer.SetReference(mainRef)
	repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{bare}})
	repo.Push(&git.PushOptions{RemoteName: "origin", RefSpecs: []config.RefSpec{"refs/heads/main:refs/heads/main"}})
	return bare
}

func postTestConn(t *testing.T, srv *Server, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/git/test-connection", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestHandleTestGitConnection_OK(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	bare := makeBareRepoWithMain(t)
	rr := postTestConn(t, srv, cookie, map[string]any{
		"remote": bare,
		"branch": "main",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp testConnResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.OK || resp.Code != "ok" || !resp.BranchFound {
		t.Fatalf("resp %+v", resp)
	}
}

func TestHandleTestGitConnection_BranchMissing(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	bare := makeBareRepoWithMain(t)
	rr := postTestConn(t, srv, cookie, map[string]any{
		"remote": bare,
		"branch": "nope",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp testConnResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.OK || resp.Code != "branch_missing" {
		t.Fatalf("resp %+v", resp)
	}
}

func TestHandleTestGitConnection_EmptyRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	dir := t.TempDir()
	bare := filepath.Join(dir, "empty.git")
	if _, err := git.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	rr := postTestConn(t, srv, cookie, map[string]any{
		"remote": bare,
		"branch": "main",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp testConnResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.OK || resp.Code != "empty_repo" {
		t.Fatalf("resp %+v", resp)
	}
	if resp.Message != testConnMessages["empty_repo"] {
		t.Fatalf("message %q", resp.Message)
	}
}

func TestHandleTestGitConnection_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := superAdminCookie(t, srv.jwt)
	rr := postTestConn(t, srv, cookie, map[string]any{
		"remote": filepath.Join(t.TempDir(), "does-not-exist.git"),
		"branch": "main",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp testConnResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.OK || resp.Code == "ok" {
		t.Fatalf("expected non-ok, got %+v", resp)
	}
}

func TestHandleTestGitConnection_NonAdmin(t *testing.T) {
	srv, st := newTestServer(t)
	jwtMgr := auth.NewJWTManager("test-secret", time.Hour)
	srv.jwt = jwtMgr
	if _, err := st.CreateUser("regular", "hashed", "manage", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
	tok, _ := jwtMgr.Generate(2, "regular", "admin")
	cookie := &http.Cookie{Name: "session", Value: tok}
	rr := postTestConn(t, srv, cookie, map[string]any{"remote": "x", "branch": "main"})
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401/403, got %d body=%s", rr.Code, rr.Body.String())
	}
}
