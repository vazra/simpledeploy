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
// A pulled sidecar that SimpleDeploy re-renders from the DB after importing
// it (e.g. a hand-formatted simpledeploy.yml saved back in SimpleDeploy's
// format) carries no local edit. Right after an import, git sync records the
// SHA-256 of that render; a write with exactly those bytes is the re-render,
// whenever it lands, and any other write is a local edit (skipPullRewrites).
// Re-renders are committed separately under pulledRewriteSubject, and on
// conflict the rebase takes the remote side for them, as it does for an
// access restore (see replayingPullFollowUp). Otherwise such a commit, which
// stays unpushed in pull-only mode, would override every later remote edit
// to that file. The records are kept in memory only: after a restart such
// files are committed as ordinary local changes.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/vazra/simpledeploy/internal/fsutil"
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
			delete(g.rewriteHash, rel)
		}
	}
}

// recordPullRenders records, for each app whose sidecar a pull just
// imported, the SHA-256 of that sidecar as SimpleDeploy renders it from the
// DB now.
func (g *Syncer) recordPullRenders(slugs []string) {
	if g.cs == nil {
		return
	}
	for _, slug := range slugs {
		b, err := g.cs.RenderAppSidecar(slug)
		if err != nil {
			log.Printf("[gitsync] render pulled sidecar %s: %v", slug, err)
			continue
		}
		sum := sha256.Sum256(b)
		g.mu.Lock()
		if g.rewriteHash == nil {
			g.rewriteHash = map[string]string{}
		}
		g.rewriteHash[slug+"/simpledeploy.yml"] = hex.EncodeToString(sum[:])
		g.mu.Unlock()
	}
}

// releaseHeld classifies the commit requests made during an import.
func (g *Syncer) releaseHeld() {
	g.mu.Lock()
	held := g.held
	g.held = nil
	g.mu.Unlock()
	for _, r := range held {
		g.EnqueueCommit(r.paths, r.reason)
	}
}

// skipPullRewrites records the paths whose file holds exactly SimpleDeploy's
// render of a pulled sidecar as pull rewrites and returns the others, which
// are local changes. ok is false when nothing is left to commit. nil (stage
// everything) passes through: staging leaves pull rewrites out.
func (g *Syncer) skipPullRewrites(paths []string) ([]string, bool) {
	if paths == nil {
		return nil, true
	}
	var keep []string
	for _, p := range paths {
		if rel, ok := g.repoRel(p); ok && g.matchesPullRender(rel) {
			g.notePullRewrites([]string{p})
			continue
		}
		g.forgetPullRewrites([]string{p})
		keep = append(keep, p)
	}
	return keep, len(keep) > 0
}

// matchesPullRender reports whether the file at rel is byte for byte the
// render recorded by recordPullRenders.
func (g *Syncer) matchesPullRender(rel string) bool {
	g.mu.Lock()
	want, ok := g.rewriteHash[rel]
	g.mu.Unlock()
	if !ok {
		return false
	}
	b, err := fsutil.ReadRegularFile(filepath.Join(g.cfg.AppsDir, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) == want
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
