package gitsync

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

var userinfoRE = regexp.MustCompile(`(https?)://[^/@\s]+:[^/@\s]+@`)

// scrubSecrets removes the HTTPS token (if non-empty) and any URL userinfo
// from s, so RawError can be safely shown to operators.
func scrubSecrets(s string, token string) string {
	if token != "" {
		s = strings.ReplaceAll(s, token, "***")
	}
	s = userinfoRE.ReplaceAllString(s, "$1://***@")
	return s
}

// RemoteCheck is the structured result of a connectivity probe.
type RemoteCheck struct {
	OK          bool   `json:"ok"`
	Code        string `json:"code"` // ok|auth_failed|not_found|branch_missing|network|unknown
	BranchFound bool   `json:"branch_found"`
	RefCount    int    `json:"ref_count"`
	RawError    string `json:"raw_error"`
}

// CheckRemote performs a single ls-remote against cfg.Remote and reports
// whether auth succeeded and whether cfg.Branch exists on the remote.
func CheckRemote(cfg Config) RemoteCheck {
	if cfg.Remote == "" {
		return RemoteCheck{Code: "unknown", RawError: scrubSecrets("remote is required", cfg.HTTPSToken)}
	}
	branch := cfg.Branch
	if branch == "" {
		branch = "main"
	}
	auth, err := buildAuth(cfg)
	if err != nil {
		return RemoteCheck{Code: "auth_failed", RawError: scrubSecrets(err.Error(), cfg.HTTPSToken)}
	}
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{cfg.Remote},
	})
	refs, err := rem.List(&git.ListOptions{Auth: auth})
	if err != nil {
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return RemoteCheck{
				OK:          true,
				Code:        "empty_repo",
				BranchFound: false,
				RefCount:    0,
			}
		}
		return RemoteCheck{
			Code:     classifyRemoteErr(err),
			RawError: scrubSecrets(err.Error(), cfg.HTTPSToken),
		}
	}
	if len(refs) == 0 {
		return RemoteCheck{
			OK:          true,
			Code:        "empty_repo",
			BranchFound: false,
			RefCount:    0,
		}
	}
	target := plumbing.NewBranchReferenceName(branch)
	found := false
	for _, r := range refs {
		if r.Name() == target {
			found = true
			break
		}
	}
	if !found {
		return RemoteCheck{
			Code:        "branch_missing",
			BranchFound: false,
			RefCount:    len(refs),
			RawError:    scrubSecrets(fmt.Sprintf("branch %q not found on remote", branch), cfg.HTTPSToken),
		}
	}
	return RemoteCheck{
		OK:          true,
		Code:        "ok",
		BranchFound: true,
		RefCount:    len(refs),
	}
}

func classifyRemoteErr(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, transport.ErrAuthenticationRequired),
		errors.Is(err, transport.ErrAuthorizationFailed):
		return "auth_failed"
	case errors.Is(err, transport.ErrRepositoryNotFound):
		return "not_found"
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return "network"
	}
	return "unknown"
}
