package gitsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRemoteURL(t *testing.T) {
	allowed := []string{
		"https://github.com/owner/repo.git",
		"http://gitea.local:3000/owner/repo.git",
		"HTTPS://github.com/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"ssh://deploy@[2001:db8::1]:2222/srv/repo.git",
		"git://example.com/repo.git",
		"file:///srv/git/repo.git",
		"file://unused",
		"/srv/git/repo.git",
		"git@github.com:owner/repo.git",
		"deploy@git.example.com:infra/apps.git",
		"git@[2001:db8::1]:repo.git",
	}
	for _, r := range allowed {
		if err := ValidateRemoteURL(r); err != nil {
			t.Errorf("ValidateRemoteURL(%q) = %v, want nil", r, err)
		}
	}

	rejected := []string{
		"",
		"ext::sh -c touch% /tmp/pwned",
		"fd::3",
		"fd::0,1/foo",
		"ssh::host/repo",
		"-uhttps://evil",
		"--upload-pack=touch /tmp/pwned",
		"ftp://example.com/repo.git",
		"ftps://example.com/repo.git",
		"rsync://example.com/repo.git",
		"git+ssh://example.com/repo.git",
		"ssh://-oProxyCommand=touch%20pwned/repo",
		"https://-evil.example.com/repo.git",
		"ssh://-user@host/repo.git",
		"-oProxyCommand=x@host:repo",
		"git@-oProxyCommand=x:repo",
		"git@host:-repo",
		"git@host:",
		"relative/path/repo.git",
		"repo",
		"https://example.com/repo\n.git",
		"https://example.com/re\x00po.git",
		"https:///nohost",
	}
	for _, r := range rejected {
		if err := ValidateRemoteURL(r); err == nil {
			t.Errorf("ValidateRemoteURL(%q) = nil, want error", r)
		}
	}
}

func TestValidateRemoteURLDoesNotEchoCredentials(t *testing.T) {
	err := ValidateRemoteURL("ftp://user:s3cr3t-token@example.com/repo.git")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cr3t-token") {
		t.Fatalf("error leaks credentials: %v", err)
	}
	err = ValidateRemoteURL("user:s3cr3t-token@example.com/repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cr3t-token") {
		t.Fatalf("error leaks credentials: %v", err)
	}
}

func TestValidateBranch(t *testing.T) {
	allowed := []string{"main", "master", "release/v1.2", "feature_x-1", "a/b/c", "v1.0"}
	for _, b := range allowed {
		if err := ValidateBranch(b); err != nil {
			t.Errorf("ValidateBranch(%q) = %v, want nil", b, err)
		}
	}
	rejected := []string{
		"",
		"-main",
		"--upload-pack=x",
		"main..evil",
		"..",
		"/main",
		"main/",
		"a//b",
		".hidden",
		"a/.hidden",
		"main.lock",
		"main.",
		"main branch",
		"main;rm",
		"main~1",
		"main^",
		"ma:in",
		"@{-1}",
		"refs/heads/ma*n",
		strings.Repeat("a", 256),
	}
	for _, b := range rejected {
		if err := ValidateBranch(b); err == nil {
			t.Errorf("ValidateBranch(%q) = nil, want error", b)
		}
	}
}

func TestNewRejectsInvalidRemoteAndBranch(t *testing.T) {
	base := Config{Enabled: true, AppsDir: t.TempDir(), Remote: "file:///srv/repo.git", Branch: "main"}
	if _, err := New(base, nil, nil, nil); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	badRemote := base
	badRemote.Remote = "ext::sh -c id"
	if _, err := New(badRemote, nil, nil, nil); err == nil {
		t.Error("New accepted ext:: remote")
	}

	badBranch := base
	badBranch.Branch = "--orphan"
	if _, err := New(badBranch, nil, nil, nil); err == nil {
		t.Error("New accepted branch starting with '-'")
	}
}

func TestCheckRemoteRejectsInvalidConfig(t *testing.T) {
	res := CheckRemote(Config{Remote: "ext::sh -c id", Branch: "main"})
	if res.OK || res.Code != "invalid_config" {
		t.Fatalf("ext:: remote: got %+v, want invalid_config", res)
	}
	res = CheckRemote(Config{Remote: "file:///srv/repo.git", Branch: "-x"})
	if res.OK || res.Code != "invalid_config" {
		t.Fatalf("bad branch: got %+v, want invalid_config", res)
	}
	if err := ValidateRemote(Config{Remote: "fd::3"}); err == nil {
		t.Fatal("ValidateRemote accepted fd:: remote")
	}
}

func TestBuildAuthRejectsInvalidRemote(t *testing.T) {
	if _, err := buildAuth(Config{Remote: "ext::sh -c id"}); err == nil {
		t.Fatal("buildAuth accepted ext:: remote")
	}
}

func TestSSHRemoteDetection(t *testing.T) {
	cases := []struct {
		remote string
		ssh    bool
		user   string
	}{
		{"git@github.com:owner/repo.git", true, "git"},
		{"deploy@host:infra.git", true, "deploy"},
		{"ssh://host/repo.git", true, "git"},
		{"ssh://deploy@host/repo.git", true, "deploy"},
		{"https://github.com/owner/repo.git", false, ""},
		{"file:///srv/repo.git", false, ""},
		{"/srv/repo.git", false, ""},
	}
	for _, c := range cases {
		if got := isSSHRemote(c.remote); got != c.ssh {
			t.Errorf("isSSHRemote(%q) = %v, want %v", c.remote, got, c.ssh)
		}
		if c.ssh {
			if got := sshUser(c.remote); got != c.user {
				t.Errorf("sshUser(%q) = %q, want %q", c.remote, got, c.user)
			}
		}
	}
}

// TestGitExecBlocksExtAndFdTransports: gitExec must refuse ext:: (arbitrary
// command) and fd:: transports even if a remote slipped past validation.
func TestGitExecBlocksExtAndFdTransports(t *testing.T) {
	dir := t.TempDir()
	if out, err := gitExec(dir, "init", "-q"); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	marker := filepath.Join(t.TempDir(), "pwned")
	out, err := gitExec(dir, "ls-remote", "ext::touch "+marker)
	if err == nil {
		t.Fatalf("ext:: ls-remote succeeded: %s", out)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("ext:: transport executed a command")
	}
	if !strings.Contains(string(out), "not allowed") {
		t.Logf("ext:: output: %s", out)
	}

	out, err = gitExec(dir, "ls-remote", "fd::0")
	if err == nil {
		t.Fatalf("fd:: ls-remote succeeded: %s", out)
	}
}

func TestScrubbedGitEnvOverridesAllowProtocol(t *testing.T) {
	t.Setenv("GIT_ALLOW_PROTOCOL", "ext:file")
	env := scrubbedGitEnv()
	var got []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_ALLOW_PROTOCOL=") {
			got = append(got, kv)
		}
	}
	if len(got) != 1 || got[0] != "GIT_ALLOW_PROTOCOL="+gitAllowedProtocols {
		t.Fatalf("GIT_ALLOW_PROTOCOL entries = %v", got)
	}
}
