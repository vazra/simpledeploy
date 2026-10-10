package gitsync

// pending.go commits uncommitted changes to managed paths before a rebase.
//
// git refuses to rebase over uncommitted changes to tracked files, and over
// untracked files the incoming commits would create. Dashboard edits are
// normally committed by doCommit (and only pushed when auto-push is on), but
// some writes never get a commit: writes made while a pull is being
// imported (suppress on), dropped requests, hand edits. commitPending folds
// them into local bot commits so the rebase can run and the server-wins
// conflict policy decides what is kept.
//
// Files SimpleDeploy writes while importing a pull (the sidecar write hook
// fires while suppress is on, e.g. a pulled sidecar re-rendered from the
// DB) carry no local edit. They are committed separately under
// pulledRewriteSubject, and on conflict the rebase takes the remote side for
// them, as it does for an access restore (see replayingPullFollowUp).
// Otherwise such a commit, which stays unpushed in pull-only mode, would
// override every later remote edit to that file. The set is kept in memory
// only: after a restart such files are committed as ordinary local changes.

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	// syncCommitSubject is the subject of commits holding local config changes.
	syncCommitSubject = "chore(simpledeploy): sync config"

	// pulledRewriteSubject is the subject of the local commit holding files
	// SimpleDeploy rewrote while importing a pull.
	pulledRewriteSubject = "chore(simpledeploy): rewrite pulled config"
)

// pendingCommits reports what commitPending committed.
type pendingCommits struct {
	local   bool // local changes were committed
	rewrite bool // files rewritten while importing a pull were committed
}

// commitPending commits every uncommitted change to a managed path (edits,
// deletions, new untracked files) so a rebase onto origin can run. Nothing
// is pushed. A rebase left stopped by an earlier crash is aborted first.
func (g *Syncer) commitPending() (pendingCommits, error) {
	var res pendingCommits
	if rebaseInProgress(g.cfg.AppsDir) {
		log.Printf("[gitsync] aborting a rebase left in progress in %s", g.cfg.AppsDir)
		if out, err := gitExec(g.cfg.AppsDir, "rebase", "--abort"); err != nil {
			return res, fmt.Errorf("abort leftover rebase: %w\n%s", err, bytes.TrimSpace(out))
		}
	}
	paths, err := pendingManagedPaths(g.cfg.AppsDir)
	if err != nil {
		return res, err
	}
	rewrites := g.pullRewriteSnapshot()
	var local, rewritten []string
	for _, p := range paths {
		if rewrites[p] {
			rewritten = append(rewritten, p)
		} else {
			local = append(local, p)
		}
	}
	if len(rewritten) > 0 {
		if res.rewrite, err = g.commitPathsGit(rewritten, pulledRewriteSubject,
			"files rewritten by SimpleDeploy after applying a pull"); err != nil {
			return res, err
		}
	}
	if len(local) > 0 {
		if res.local, err = g.commitPathsGit(local, syncCommitSubject,
			"uncommitted local changes, committed before applying a pull"); err != nil {
			return res, err
		}
	}
	g.dropPullRewrites(rewrites)
	if res.local || res.rewrite {
		if sha, err := gitHead(g.cfg.AppsDir); err == nil {
			g.mu.Lock()
			g.headSHA = sha
			g.mu.Unlock()
		}
	}
	return res, nil
}

// pendingManagedPaths lists repo-relative managed paths (isAllowedPath) that
// differ from HEAD: modified, deleted, staged or untracked, as system git
// sees them. Symlinks, and paths in a symlinked app folder, are left out:
// git sync never commits them.
func pendingManagedPaths(appsDir string) ([]string, error) {
	out, err := gitExec(appsDir, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return nil, fmt.Errorf("git status: %w\n%s", err, bytes.TrimSpace(out))
	}
	var paths []string
	for _, entry := range strings.Split(string(out), "\x00") {
		if len(entry) < 4 {
			continue
		}
		rel := entry[3:] // "XY path"
		if !isAllowedPath(filepath.FromSlash(rel)) || onSymlink(appsDir, rel) {
			continue
		}
		paths = append(paths, rel)
	}
	return paths, nil
}

// onSymlink reports whether rel, or the app folder holding it, is a symlink.
func onSymlink(appsDir, rel string) bool {
	isLink := func(p string) bool {
		fi, err := os.Lstat(filepath.Join(appsDir, filepath.FromSlash(p)))
		return err == nil && fi.Mode()&os.ModeSymlink != 0
	}
	if dir, _, ok := strings.Cut(rel, "/"); ok && isLink(dir) {
		return true
	}
	return isLink(rel)
}

// commitPathsGit stages relPaths (edits, deletions, new files) and commits
// only those paths, without pushing. Returns false when they hold no change.
// It uses system git, like the rebase, so the commit sees the tree the way
// the rebase does (with core.symlinks=false a tracked symlink checked out as
// a plain file is unchanged, where go-git would record a mode change).
func (g *Syncer) commitPathsGit(relPaths []string, subject, reason string) (bool, error) {
	dir := g.cfg.AppsDir
	add := append([]string{"--literal-pathspecs", "add", "-A", "--"}, relPaths...)
	if out, err := gitExec(dir, add...); err != nil {
		return false, fmt.Errorf("git add: %w\n%s", err, bytes.TrimSpace(out))
	}
	diff := append([]string{"--literal-pathspecs", "diff", "--cached", "--quiet", "--"}, relPaths...)
	if _, err := gitExec(dir, diff...); err == nil {
		return false, nil
	}
	commit := []string{
		"--literal-pathspecs",
		"-c", "user.name=" + g.cfg.authorName(),
		"-c", "user.email=" + g.cfg.authorEmail(),
		"-c", "commit.gpgsign=false",
		"commit", "-q", "--no-verify", "-m", buildCommitMessage(subject, reason), "--",
	}
	commit = append(commit, relPaths...)
	if out, err := gitExec(dir, commit...); err != nil {
		return false, fmt.Errorf("git commit: %w\n%s", err, bytes.TrimSpace(out))
	}
	return true, nil
}

// repoRel converts a path under apps_dir (absolute, or already relative) to
// a slash-separated repo-relative managed path.
func (g *Syncer) repoRel(p string) (string, bool) {
	rel := p
	if filepath.IsAbs(p) {
		r, err := filepath.Rel(g.cfg.AppsDir, p)
		if err != nil {
			return "", false
		}
		rel = r
	}
	if strings.HasPrefix(rel, "..") || !isAllowedPath(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// notePullRewrites records managed paths written while a pull was being
// imported. nil paths (stage-everything requests) name nothing.
func (g *Syncer) notePullRewrites(paths []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range paths {
		rel, ok := g.repoRel(p)
		if !ok {
			continue
		}
		if g.pullRewrites == nil {
			g.pullRewrites = map[string]bool{}
		}
		g.pullRewrites[rel] = true
	}
}

// forgetPullRewrites drops paths that a local change has since written.
func (g *Syncer) forgetPullRewrites(paths []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range paths {
		if rel, ok := g.repoRel(p); ok {
			delete(g.pullRewrites, rel)
		}
	}
}

func (g *Syncer) isPullRewrite(rel string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pullRewrites[rel]
}

func (g *Syncer) pullRewriteSnapshot() map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]bool, len(g.pullRewrites))
	for k := range g.pullRewrites {
		out[k] = true
	}
	return out
}

func (g *Syncer) dropPullRewrites(keys map[string]bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for k := range keys {
		delete(g.pullRewrites, k)
	}
}
