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

	if err := d.Deploy(context.Background(), app); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if !mock.HasCall("docker", "compose", "up", "-d", "--force-recreate", "--remove-orphans") {
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

	err := d.Deploy(context.Background(), app)
	if err == nil {
		t.Fatal("expected error from Deploy")
	}
	if !strings.Contains(err.Error(), "compose failed") {
		t.Errorf("expected 'compose failed' in error, got: %v", err)
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
}
