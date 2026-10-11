package deployer

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
)

// portInUse is what `docker compose up` prints when a host port is taken.
const portInUse = `time="2026-01-01T00:00:00Z" level=warning msg="/apps/myapp/docker-compose.yml: the attribute ` + "`version`" + ` is obsolete"
 Network simpledeploy-myapp_default  Creating
 Network simpledeploy-myapp_default  Created
 Container simpledeploy-myapp-web-1  Creating
 Container simpledeploy-myapp-web-1  Created
 Container simpledeploy-myapp-web-1  Starting
Error response from daemon: driver failed programming external connectivity on endpoint simpledeploy-myapp-web-1: Bind for 127.0.0.1:8080 failed: port is already allocated
`

// failOn makes runner calls whose args contain sub fail with stderr.
func failOn(sub, stderr string) func(RunCall) (string, string, error, bool) {
	return func(c RunCall) (string, string, error, bool) {
		if slices.Contains(c.Args, sub) {
			return "", stderr, errors.New("exit status 1"), true
		}
		return "", "", nil, false
	}
}

func TestDeployErrorSaysWhyComposeFailed(t *testing.T) {
	audit := &fakeAudit{}
	d := &Deployer{runner: &MockRunner{Respond: failOn("up", portInUse)}, audit: audit}

	res := d.Deploy(context.Background(), testApp(t, "myapp"))
	if res.Err == nil {
		t.Fatal("expected error")
	}
	msg := res.Err.Error()
	if !strings.Contains(msg, "port is already allocated") {
		t.Errorf("error = %q, want the compose error line", msg)
	}
	if strings.Contains(msg, "Starting") || strings.Contains(msg, "level=warning") {
		t.Errorf("error = %q, want no progress or warning lines", msg)
	}
	var ce *ComposeError
	if !errors.As(res.Err, &ce) || ce.Op != "compose up" {
		t.Errorf("error = %#v, want *ComposeError for compose up", res.Err)
	}
	evt, ok := audit.last()
	if !ok || evt.Action != "deploy_failed" || !strings.Contains(evt.Error, "port is already allocated") {
		t.Errorf("audit = %+v, want deploy_failed with the compose error", evt)
	}
}

func TestRollbackAndPullErrorsSayWhy(t *testing.T) {
	denied := " web Pulling\n web Error pull access denied for acme/private, repository does not exist or may require 'docker login'\nError response from daemon: pull access denied for acme/private, repository does not exist or may require 'docker login'\n"

	d := &Deployer{runner: &MockRunner{Respond: failOn("up", denied)}}
	res := d.RollbackDeploy(context.Background(), testApp(t, "myapp"), 2, nil)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "pull access denied for acme/private") {
		t.Errorf("rollback error = %v, want the pull error", res.Err)
	}

	d = &Deployer{runner: &MockRunner{Respond: failOn("pull", denied)}}
	res = d.Pull(context.Background(), testApp(t, "myapp"), nil)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "pull access denied for acme/private") {
		t.Errorf("pull error = %v, want the pull error", res.Err)
	}
}

func TestDeployErrorRedactsRegistryCredentials(t *testing.T) {
	auth := RegistryAuth{URL: "registry.example.com", Username: "bot", Password: "hunter2-s3cret"}
	b64 := base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + auth.Password))
	stderr := "Error response from daemon: login failed for bot:hunter2-s3cret (auth " + b64 + ")\n"
	d := &Deployer{runner: &MockRunner{Respond: failOn("up", stderr)}}

	res := d.Deploy(context.Background(), testApp(t, "myapp"), auth)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	for _, s := range []string{res.Err.Error(), res.Output} {
		if strings.Contains(s, auth.Password) || strings.Contains(s, b64) {
			t.Errorf("credentials leaked: %q", s)
		}
	}
	if !strings.Contains(res.Err.Error(), "login failed") {
		t.Errorf("error = %q, want the rest of the message kept", res.Err)
	}
}

func TestFailureDetail(t *testing.T) {
	long := "Error response from daemon: " + strings.Repeat("x", 3000) + " the end"
	cases := []struct {
		name, stderr, want string
	}{
		{"port in use", portInUse, "Error response from daemon: driver failed programming external connectivity on endpoint simpledeploy-myapp-web-1: Bind for 127.0.0.1:8080 failed: port is already allocated"},
		{"pull error printed twice", " web Pulling\n web Error pull access denied for x\nError response from daemon: pull access denied for x\n", "Error response from daemon: pull access denied for x"},
		{"unhealthy dependency", " Container a-db-1  Waiting\n Container a-db-1  Error\ndependency failed to start: container a-db-1 is unhealthy\n", "dependency failed to start: container a-db-1 is unhealthy"},
		{"last lines kept in order", "one\ntwo\nthree\nfour\n", "two\nthree\nfour"},
		{"only progress", " Container a-web-1  Starting\n", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		if got := failureDetail(tc.stderr); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	got := failureDetail(long)
	if len(got) > maxDetailBytes+len("...") || !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, " the end") {
		t.Errorf("long: got %d bytes %q..., want capped with the end kept", len(got), got[:20])
	}
}
