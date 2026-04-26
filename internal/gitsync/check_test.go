package gitsync

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

func TestClassifyRemoteErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "ok"},
		{"auth required", transport.ErrAuthenticationRequired, "auth_failed"},
		{"auth forbidden", transport.ErrAuthorizationFailed, "auth_failed"},
		{"not found", transport.ErrRepositoryNotFound, "not_found"},
		{"net dns", &net.DNSError{Err: "no such host", Name: "x"}, "network"},
		{"unknown", errors.New("boom"), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyRemoteErr(tc.err)
			if got != tc.want {
				t.Fatalf("classifyRemoteErr(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestCheckRemote_BranchPresence(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	if _, err := git.PlainInit(bare, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainInit(work, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()
	if err := os.WriteFile(filepath.Join(work, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt.Add("README")
	_, err = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "x", Email: "x@x", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	headRef, _ := repo.Head()
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), headRef.Hash())
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{bare}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Push(&git.PushOptions{RemoteName: "origin", RefSpecs: []config.RefSpec{"refs/heads/main:refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}

	t.Run("branch_found", func(t *testing.T) {
		got := CheckRemote(Config{Remote: bare, Branch: "main"})
		if !got.OK || got.Code != "ok" || !got.BranchFound {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("branch_missing", func(t *testing.T) {
		got := CheckRemote(Config{Remote: bare, Branch: "does-not-exist"})
		if got.OK || got.Code != "branch_missing" {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestCheckRemote_ScrubsToken(t *testing.T) {
	res := CheckRemote(Config{
		Remote:     "/tmp/definitely-not-a-real-repo-xyz.git",
		Branch:     "main",
		HTTPSToken: "supersecrettoken123",
	})
	if res.OK {
		t.Fatalf("expected failure")
	}
	if strings.Contains(res.RawError, "supersecrettoken123") {
		t.Fatalf("RawError leaked token: %q", res.RawError)
	}
}
