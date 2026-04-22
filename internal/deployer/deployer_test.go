package deployer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestDeployCallsComposeUp(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}

	app := &compose.AppConfig{
		Name:        "myapp",
		ComposePath: "/apps/myapp/docker-compose.yml",
	}

	if result := d.Deploy(context.Background(), app); result.Err != nil {
		t.Fatalf("Deploy: %v", result.Err)
	}

	if !mock.HasCall("docker", "compose", "up", "-d", "--remove-orphans") {
		t.Errorf("expected docker compose up call, got: %+v", mock.Calls)
	}
	if !mock.HasCall("docker", "-p", "simpledeploy-myapp") {
		t.Errorf("expected project name simpledeploy-myapp, got: %+v", mock.Calls)
	}
}

func TestTeardownCallsComposeDown(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}

	if err := d.Teardown(context.Background(), "myapp"); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if !mock.HasCall("docker", "compose", "down", "--remove-orphans") {
		t.Errorf("expected docker compose down call, got: %+v", mock.Calls)
	}
	if !mock.HasCall("docker", "-p", "simpledeploy-myapp") {
		t.Errorf("expected project name simpledeploy-myapp, got: %+v", mock.Calls)
	}
}

func TestDeployPropagatesError(t *testing.T) {
	mock := &MockRunner{Err: fmt.Errorf("compose failed")}
	d := &Deployer{runner: mock}

	app := &compose.AppConfig{
		Name:        "myapp",
		ComposePath: "/apps/myapp/docker-compose.yml",
	}

	result := d.Deploy(context.Background(), app)
	if result.Err == nil {
		t.Fatal("expected error from Deploy")
	}
	if !strings.Contains(result.Err.Error(), "compose failed") {
		t.Errorf("expected 'compose failed' in error, got: %v", result.Err)
	}
}

func TestNewVerifiesComposeAvailable(t *testing.T) {
	mock := &MockRunner{}
	_, err := New(mock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if !mock.HasCall("docker", "compose", "version") {
		t.Error("expected docker compose version check")
	}
}

func TestNewFailsWhenComposeUnavailable(t *testing.T) {
	mock := &MockRunner{Err: fmt.Errorf("not found")}
	_, err := New(mock)
	if err == nil {
		t.Fatal("expected error from New when compose unavailable")
	}
	if !strings.Contains(err.Error(), "docker compose not available") {
		t.Errorf("expected 'docker compose not available' in error, got: %v", err)
	}
}

func TestRestartCallsComposeRestart(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/apps/myapp/docker-compose.yml"}
	if result := d.Restart(context.Background(), app); result.Err != nil {
		t.Fatalf("Restart: %v", result.Err)
	}
	if !mock.HasCall("docker", "compose", "restart") {
		t.Errorf("expected compose restart, got: %+v", mock.Calls)
	}
}

func TestStopCallsComposeStop(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	if err := d.Stop(context.Background(), "myapp"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !mock.HasCall("docker", "compose", "stop") {
		t.Errorf("expected compose stop, got: %+v", mock.Calls)
	}
}

func TestStartCallsComposeStart(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	if err := d.Start(context.Background(), "myapp"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !mock.HasCall("docker", "compose", "start") {
		t.Errorf("expected compose start, got: %+v", mock.Calls)
	}
}

func TestPullCallsPullThenUp(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/apps/myapp/docker-compose.yml"}
	if result := d.Pull(context.Background(), app, nil); result.Err != nil {
		t.Fatalf("Pull: %v", result.Err)
	}
	if !mock.HasCall("docker", "compose", "pull") {
		t.Errorf("expected compose pull, got: %+v", mock.Calls)
	}
	if !mock.HasCall("docker", "compose", "up", "-d") {
		t.Errorf("expected compose up after pull, got: %+v", mock.Calls)
	}
}

func TestPullWithAuth(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/tmp/docker-compose.yml"}

	auths := []RegistryAuth{
		{URL: "ghcr.io", Username: "user", Password: "pass"},
	}
	result := d.Pull(context.Background(), app, auths)
	if result.Err != nil {
		t.Fatalf("Pull: %v", result.Err)
	}

	found := false
	for _, c := range mock.Calls {
		for _, arg := range c.Args {
			if arg == "--config" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected --config flag in docker pull call")
	}
}

func TestPullWithoutAuth(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/tmp/docker-compose.yml"}

	result := d.Pull(context.Background(), app, nil)
	if result.Err != nil {
		t.Fatalf("Pull: %v", result.Err)
	}

	for _, c := range mock.Calls {
		for _, arg := range c.Args {
			if arg == "--config" {
				t.Error("unexpected --config flag when no auths")
			}
		}
	}
}

func TestScaleCallsComposeUpWithScale(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/apps/myapp/docker-compose.yml"}
	if err := d.Scale(context.Background(), app, map[string]int{"web": 3}); err != nil {
		t.Fatalf("Scale: %v", err)
	}
	if !mock.HasCall("docker", "compose", "up", "--no-recreate", "--scale") {
		t.Errorf("expected compose up --scale, got: %+v", mock.Calls)
	}
}

func TestStatusCallsComposePs(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	_, err := d.Status(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !mock.HasCall("docker", "compose", "ps", "--format", "json") {
		t.Errorf("expected compose ps call, got: %+v", mock.Calls)
	}
}
