package gitsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A rebase git refuses (uncommitted changes) must surface as an error, not
// a silent no-op.
func TestRebaseServerWinsReportsDirtyTree(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "--bare", "-b", "main", remote)
	run(root, "clone", remote, work)
	run(work, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(work, "add", "a.txt")
	run(work, "commit", "-m", "init")
	run(work, "push", "origin", "main")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := rebaseServerWins(work, "main")
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("err = %v, want uncommitted changes error", err)
	}
}

// An untracked local file the remote would overwrite must surface as an
// error too.
func TestRebaseServerWinsReportsUntrackedOverwrite(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	other := filepath.Join(root, "other")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "--bare", "-b", "main", remote)
	run(root, "clone", remote, work)
	run(work, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(work, "a.txt"), []byte("1\n"), 0o600)
	run(work, "add", "a.txt")
	run(work, "commit", "-m", "init")
	run(work, "push", "origin", "main")
	// A second clone adds app/new.txt on the remote.
	run(root, "clone", remote, other)
	os.MkdirAll(filepath.Join(other, "app"), 0o700)
	os.WriteFile(filepath.Join(other, "app", "new.txt"), []byte("remote\n"), 0o600)
	run(other, "add", ".")
	run(other, "commit", "-m", "remote add")
	run(other, "push", "origin", "main")
	// Locally the same path exists untracked.
	os.MkdirAll(filepath.Join(work, "app"), 0o700)
	os.WriteFile(filepath.Join(work, "app", "new.txt"), []byte("local\n"), 0o600)
	run(work, "fetch", "origin")

	_, _, err := rebaseServerWins(work, "main")
	if err == nil {
		t.Fatal("want error when the pull would overwrite an untracked file")
	}
}
