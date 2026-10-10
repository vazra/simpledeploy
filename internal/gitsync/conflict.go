package gitsync

// conflict.go handles server-wins conflict resolution during pull.
//
// go-git (v5) does not support interactive rebase or per-file conflict
// resolution (checkout --ours/--theirs) at the library level. Therefore we
// fall back to shelling out to the system `git` binary for the rebase step
// when the remote has diverged.
//
// Ours/theirs semantics during rebase vs merge:
//   - In a merge,  "ours" = local branch HEAD.
//   - In a rebase, "ours" = the upstream (remote) commits being replayed onto;
//     "theirs" = the local commits being reapplied.
//
// We want local (server) to win, so during a rebase we use `--theirs` for
// conflicted files. The exception is a local "restore access grants" commit
// (unpushed in pull-only mode): it only reverts pulled access lists, so its
// conflicts take the remote file and securePulledTree re-applies the access
// restore afterwards. Otherwise the remote's other edits to that sidecar
// (e.g. alert thresholds) would be dropped on every later pull.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// rebaseServerWins runs `git -C dir fetch origin` then rebases local commits
// on top of origin/<branch>. On conflict, takes local side (--theirs in
// rebase context). Returns the list of resolved conflicts and the new HEAD SHA.
func rebaseServerWins(appsDir, branch string) ([]Conflict, string, error) {
	// git -C <dir> rebase origin/<branch>
	// If there are conflicts, loop: checkout --theirs, add, continue.
	out, err := gitExec(appsDir, "rebase", "origin/"+branch)
	if err == nil {
		// Clean rebase, no conflicts.
		sha, shaErr := gitHead(appsDir)
		return nil, sha, shaErr
	}

	// git refuses to start when the working tree has uncommitted changes;
	// report it instead of treating it as a conflict-free no-op.
	if bytes.Contains(out, []byte("cannot rebase")) || bytes.Contains(out, []byte("commit or stash")) ||
		bytes.Contains(out, []byte("would be overwritten")) || bytes.Contains(out, []byte("could not detach HEAD")) {
		return nil, "", fmt.Errorf("gitsync: rebase refused: apps_dir has uncommitted or untracked changes that the pull would overwrite; commit or remove them (or enable auto_push) and sync again: %s", bytes.TrimSpace(out))
	}

	// Check if it's a conflict situation.
	if !bytes.Contains(out, []byte("CONFLICT")) &&
		!bytes.Contains(out, []byte("conflict")) &&
		!isRebaseConflictError(err) {
		// Abort the rebase so state is clean.
		_, _ = gitExec(appsDir, "rebase", "--abort")
		return nil, "", fmt.Errorf("gitsync: rebase: %w\n%s", err, out)
	}

	var conflicts []Conflict

	// Resolve conflicts in a loop (there may be multiple commits in the rebase).
	for {
		conflictFiles, listErr := listConflictedFiles(appsDir)
		if listErr != nil {
			_, _ = gitExec(appsDir, "rebase", "--abort")
			return nil, "", fmt.Errorf("gitsync: list conflicts: %w", listErr)
		}
		if len(conflictFiles) == 0 {
			break
		}

		takeRemote := replayingAccessRestore(appsDir)
		for _, f := range conflictFiles {
			if takeRemote {
				if err := takeUpstreamSide(appsDir, f); err != nil {
					_, _ = gitExec(appsDir, "rebase", "--abort")
					return nil, "", err
				}
				continue
			}
			// Take local side (--theirs in rebase = our server commits).
			if _, cherr := gitExec(appsDir, "checkout", "--theirs", "--", f); cherr != nil {
				_, _ = gitExec(appsDir, "rebase", "--abort")
				return nil, "", fmt.Errorf("gitsync: checkout --theirs %s: %w", f, cherr)
			}
			if _, addErr := gitExec(appsDir, "add", "--", f); addErr != nil {
				_, _ = gitExec(appsDir, "rebase", "--abort")
				return nil, "", fmt.Errorf("gitsync: add %s: %w", f, addErr)
			}
			remoteSHA, _ := gitRemoteFileSHA(appsDir, "origin/"+branch, f)
			conflicts = append(conflicts, Conflict{
				Path:        f,
				RemoteSHA:   remoteSHA,
				Description: fmt.Sprintf("server-wins: kept local version of %s", filepath.Base(f)),
			})
		}

		// Continue the rebase; skip the commit when the resolution left
		// nothing to commit (git refuses to --continue with an empty one).
		next := "--continue"
		if !hasStagedChanges(appsDir) {
			next = "--skip"
		}
		contOut, contErr := gitExec(appsDir, "rebase", next)
		if contErr == nil {
			break // done
		}
		// Still conflicts or clean finish; loop again.
		if !bytes.Contains(contOut, []byte("CONFLICT")) &&
			!bytes.Contains(contOut, []byte("conflict")) &&
			!isRebaseConflictError(contErr) {
			// Unexpected error.
			_, _ = gitExec(appsDir, "rebase", "--abort")
			return nil, "", fmt.Errorf("gitsync: rebase continue: %w\n%s", contErr, contOut)
		}
	}

	if rebaseInProgress(appsDir) {
		_, _ = gitExec(appsDir, "rebase", "--abort")
		return nil, "", errors.New("gitsync: rebase did not complete; aborted")
	}

	// Whatever path git took, the remote branch must now be part of HEAD;
	// otherwise the pull was not applied and must not be reported as done.
	if out, err := gitExec(appsDir, "merge-base", "--is-ancestor", "origin/"+branch, "HEAD"); err != nil {
		return nil, "", fmt.Errorf("gitsync: rebase did not apply origin/%s: %w\n%s", branch, err, bytes.TrimSpace(out))
	}
	sha, shaErr := gitHead(appsDir)
	return conflicts, sha, shaErr
}

// replayingAccessRestore reports whether the commit a stopped rebase is
// replaying is a simpledeploy "restore access grants" commit.
func replayingAccessRestore(appsDir string) bool {
	out, err := gitExec(appsDir, "log", "-1", "--format=%B", "REBASE_HEAD", "--")
	if err != nil {
		return false
	}
	msg := string(out)
	subject, _, _ := strings.Cut(msg, "\n")
	return strings.TrimSpace(subject) == restoreAccessSubject && isBotCommit(msg)
}

// takeUpstreamSide resolves a conflicted path with the version being rebased
// onto ("ours" during a rebase), or removes it when upstream deleted it.
func takeUpstreamSide(appsDir, f string) error {
	if _, err := gitExec(appsDir, "checkout", "--ours", "--", f); err != nil {
		if out, rmErr := gitExec(appsDir, "rm", "-q", "-f", "--", f); rmErr != nil {
			return fmt.Errorf("gitsync: take remote %s: %w\n%s", f, rmErr, out)
		}
		return nil
	}
	if out, err := gitExec(appsDir, "add", "--", f); err != nil {
		return fmt.Errorf("gitsync: add %s: %w\n%s", f, err, out)
	}
	return nil
}

// hasStagedChanges reports whether the index differs from HEAD. Errors count
// as changes so the caller falls back to `rebase --continue`.
func hasStagedChanges(appsDir string) bool {
	_, err := gitExec(appsDir, "diff", "--cached", "--quiet")
	return err != nil
}

// rebaseInProgress reports whether a rebase is still stopped in appsDir.
func rebaseInProgress(appsDir string) bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		out, err := gitExec(appsDir, "rev-parse", "--git-path", name)
		if err != nil {
			continue
		}
		p := strings.TrimSpace(string(out))
		if !filepath.IsAbs(p) {
			p = filepath.Join(appsDir, p)
		}
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// listConflictedFiles returns paths with unresolved merge conflicts. Uses -z
// so remote-controlled file names (spaces, newlines, quotes) come back
// verbatim instead of quoted or split.
func listConflictedFiles(appsDir string) ([]string, error) {
	out, _ := gitExec(appsDir, "diff", "--name-only", "-z", "--diff-filter=U")
	var files []string
	for _, l := range strings.Split(string(out), "\x00") {
		if l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

// gitRemoteFileSHA returns the blob SHA of a file on a remote ref.
func gitRemoteFileSHA(appsDir, ref, path string) (string, error) {
	out, err := gitExec(appsDir, "rev-parse", ref+":"+path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitHead returns the current HEAD commit SHA.
func gitHead(appsDir string) (string, error) {
	out, err := gitExec(appsDir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitSafetyConfig is passed as -c overrides on every system git invocation.
// ext:: runs an arbitrary command and fd:: talks to inherited file
// descriptors; neither is ever a legitimate sync remote. core.symlinks=false
// makes git check out committed symlinks as plain files, so content pulled
// from the remote cannot point managed paths outside apps_dir.
var gitSafetyConfig = []string{
	"-c", "protocol.ext.allow=never",
	"-c", "protocol.fd.allow=never",
	"-c", "core.symlinks=false",
}

// gitAllowedProtocols is exported as GIT_ALLOW_PROTOCOL, which makes git
// refuse every transport not listed, regardless of user or system config.
const gitAllowedProtocols = "file:git:http:https:ssh"

// gitExec runs git with the given args in appsDir and returns combined output.
func gitExec(appsDir string, args ...string) ([]byte, error) {
	full := make([]string, 0, len(gitSafetyConfig)+2+len(args))
	full = append(full, gitSafetyConfig...)
	full = append(full, "-C", appsDir)
	full = append(full, args...)
	cmd := exec.Command("git", full...)
	// Inherit the real environment so SSH_AUTH_SOCK etc. are available, but
	// strip variables that redirect git's view of "the repository". When this
	// process runs as (or under) a git hook, GIT_DIR/GIT_WORK_TREE/etc. are
	// exported into our env and would cause `git init` here to act on the
	// outer repo instead of appsDir, which is the gitsync test flake we hit
	// from the pre-push hook.
	cmd.Env = scrubbedGitEnv()
	out, err := cmd.CombinedOutput()
	return out, err
}

// gitEnvBlocklist names environment variables that point git at a specific
// repository, index, or worktree. Inheriting any of these into a child `git`
// process makes it operate on the wrong repo.
var gitEnvBlocklist = []string{
	"GIT_DIR",
	"GIT_INDEX_FILE",
	"GIT_WORK_TREE",
	"GIT_PREFIX",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	// Replaced with gitAllowedProtocols below.
	"GIT_ALLOW_PROTOCOL",
}

// scrubbedGitEnv returns the current process's env with redirector vars
// removed and the non-interactive overrides we always want.
func scrubbedGitEnv() []string {
	skip := make(map[string]struct{}, len(gitEnvBlocklist))
	for _, k := range gitEnvBlocklist {
		skip[k] = struct{}{}
	}
	src := os.Environ()
	out := make([]string, 0, len(src)+2)
	for _, kv := range src {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			out = append(out, kv)
			continue
		}
		if _, drop := skip[kv[:eq]]; drop {
			continue
		}
		out = append(out, kv)
	}
	out = append(out,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true",
		"GIT_ALLOW_PROTOCOL="+gitAllowedProtocols,
	)
	return out
}

// isRebaseConflictError returns true if the error message suggests a rebase conflict.
func isRebaseConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "conflict") || strings.Contains(msg, "exit status 1")
}
