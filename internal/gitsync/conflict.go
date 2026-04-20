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
// conflicted files.

import (
	"bytes"
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

		for _, f := range conflictFiles {
			// Take local side (--theirs in rebase = our server commits).
			if _, cherr := gitExec(appsDir, "checkout", "--theirs", "--", f); cherr != nil {
				_, _ = gitExec(appsDir, "rebase", "--abort")
				return nil, "", fmt.Errorf("gitsync: checkout --theirs %s: %w", f, cherr)
			}
			if _, addErr := gitExec(appsDir, "add", f); addErr != nil {
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

		// Continue the rebase.
		contOut, contErr := gitExec(appsDir, "rebase", "--continue")
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

	sha, shaErr := gitHead(appsDir)
	return conflicts, sha, shaErr
}

// listConflictedFiles returns paths with unresolved merge conflicts.
func listConflictedFiles(appsDir string) ([]string, error) {
	out, _ := gitExec(appsDir, "diff", "--name-only", "--diff-filter=U")
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
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

// gitExec runs git with the given args in appsDir and returns combined output.
func gitExec(appsDir string, args ...string) ([]byte, error) {
	// We must set GIT_AUTHOR_* and GIT_COMMITTER_* for --continue to succeed
	// on machines without global git config.
	cmd := exec.Command("git", append([]string{"-C", appsDir}, args...)...)
	// Inherit the real environment so SSH_AUTH_SOCK etc. are available.
	// Override values that must be controlled.
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true", // non-interactive
	)
	out, err := cmd.CombinedOutput()
	return out, err
}

// isRebaseConflictError returns true if the error message suggests a rebase conflict.
func isRebaseConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "conflict") || strings.Contains(msg, "exit status 1")
}
