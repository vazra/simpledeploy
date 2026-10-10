package gitsync

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Remote URL and branch validation. Both values end up on the command line of
// the system git binary (initial push, rebase) and in go-git transports, so
// they are restricted to well-known transports and conservative ref names.

var (
	// transportHelperRE matches git's "<transport>::<address>" syntax, which
	// selects remote helpers such as ext:: (runs an arbitrary command) and
	// fd::. Never allowed.
	transportHelperRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+.-]*::`)

	// scpLikeRE matches scp-like ssh remotes: user@host:path. host may be a
	// bracketed IPv6 literal.
	scpLikeRE = regexp.MustCompile(`^([^@/:\s]+)@(\[[^\]/\s]+\]|[^@/:\s\[\]]+):(.+)$`)

	branchRE = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// allowedRemoteSchemes lists the URL schemes accepted for the remote.
var allowedRemoteSchemes = map[string]bool{
	"https": true,
	"http":  true,
	"ssh":   true,
	"git":   true,
	"file":  true,
}

// ValidateRemoteURL accepts https://, http://, ssh://, git:// and file://
// URLs, absolute local paths, and scp-like ssh remotes (user@host:path). It
// rejects git transport helpers (ext::, fd::, ...), other schemes, control
// characters, and any remote, user or host starting with '-' (which git or ssh
// could parse as an option). Error messages never echo the remote, since it
// may carry credentials.
func ValidateRemoteURL(remote string) error {
	if remote == "" {
		return errors.New("remote is required")
	}
	for _, r := range remote {
		if r < 0x20 || r == 0x7f {
			return errors.New("remote URL must not contain control characters")
		}
	}
	if strings.HasPrefix(remote, "-") {
		return errors.New("remote URL must not start with '-'")
	}
	if transportHelperRE.MatchString(remote) {
		return errors.New("remote URL uses the git transport helper syntax (<transport>::<address>), which is not allowed; use https://, http://, ssh://, git://, file://, an absolute local path, or user@host:path")
	}

	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return errors.New("remote URL is not a valid URL")
		}
		scheme := strings.ToLower(u.Scheme)
		if !allowedRemoteSchemes[scheme] {
			return fmt.Errorf("remote URL scheme %q is not allowed; use https, http, ssh, git or file", u.Scheme)
		}
		host := u.Hostname()
		if host == "" && scheme != "file" {
			return errors.New("remote URL is missing a host")
		}
		if strings.HasPrefix(host, "-") {
			return errors.New("remote URL host must not start with '-'")
		}
		if u.User != nil && strings.HasPrefix(u.User.Username(), "-") {
			return errors.New("remote URL user must not start with '-'")
		}
		return nil
	}

	// Absolute local path (same transport as file://).
	if strings.HasPrefix(remote, "/") {
		return nil
	}

	if m := scpLikeRE.FindStringSubmatch(remote); m != nil {
		user, host, path := m[1], m[2], m[3]
		if strings.HasPrefix(user, "-") || strings.HasPrefix(host, "-") || strings.HasPrefix(path, "-") {
			return errors.New("remote URL user, host and path must not start with '-'")
		}
		return nil
	}

	return errors.New("unsupported remote URL; use https://, http://, ssh://, git://, file://, an absolute local path, or user@host:path")
}

// ValidateBranch accepts conservative branch names: letters, digits, '.',
// '_', '/' and '-', with no leading '-', no '..', no empty or dot-prefixed
// path components, and no '.lock' suffix.
func ValidateBranch(branch string) error {
	if branch == "" {
		return errors.New("branch is required")
	}
	if len(branch) > 255 {
		return errors.New("branch name is too long (max 255 characters)")
	}
	if !branchRE.MatchString(branch) {
		return fmt.Errorf("branch name %q may only contain letters, digits, '.', '_', '/' and '-'", branch)
	}
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("branch name %q must not start with '-'", branch)
	}
	if strings.Contains(branch, "..") {
		return fmt.Errorf("branch name %q must not contain '..'", branch)
	}
	if strings.HasSuffix(branch, ".") {
		return fmt.Errorf("branch name %q must not end with '.'", branch)
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" {
			return fmt.Errorf("branch name %q must not start or end with '/' or contain '//'", branch)
		}
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return fmt.Errorf("branch name %q has an invalid component %q", branch, part)
		}
	}
	return nil
}

// Validate checks the remote URL and branch name without touching the network.
func (c *Config) Validate() error {
	if err := ValidateRemoteURL(c.Remote); err != nil {
		return fmt.Errorf("gitsync: %w", err)
	}
	if err := ValidateBranch(c.branch()); err != nil {
		return fmt.Errorf("gitsync: %w", err)
	}
	return nil
}

// isSSHRemote reports whether remote uses the ssh transport (ssh:// URL or
// scp-like user@host:path).
func isSSHRemote(remote string) bool {
	if strings.HasPrefix(strings.ToLower(remote), "ssh://") {
		return true
	}
	return !strings.Contains(remote, "://") && scpLikeRE.MatchString(remote)
}

// sshUser returns the user embedded in an ssh remote, defaulting to "git".
func sshUser(remote string) string {
	if strings.Contains(remote, "://") {
		if u, err := url.Parse(remote); err == nil && u.User != nil && u.User.Username() != "" {
			return u.User.Username()
		}
		return "git"
	}
	if m := scpLikeRE.FindStringSubmatch(remote); m != nil && m[1] != "" {
		return m[1]
	}
	return "git"
}
