package deployer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

func dockerAvailable() bool {
	cmd := exec.Command("docker", "compose", "version")
	return cmd.Run() == nil
}

func TestIntegrationDeployAndTeardown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !dockerAvailable() {
		t.Skip("docker compose not available")
	}

	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "docker-compose.yml")
	os.WriteFile(composeFile, []byte("services:\n  web:\n    image: nginx:alpine\n    ports:\n      - \"18080:80\"\n"), 0644)

	d, err := New(&ExecRunner{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	app := &compose.AppConfig{
		Name:        "integration-test",
		ComposePath: composeFile,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := d.Deploy(ctx, app); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	// Ensure cleanup on failure
	defer d.Teardown(context.Background(), "integration-test")

	out, _, err := (&ExecRunner{}).Run(ctx, "docker", "compose", "-p", "simpledeploy-integration-test", "ps", "--format", "json")
	if err != nil {
		t.Fatalf("compose ps: %v", err)
	}
	if out == "" {
		t.Error("expected running containers after deploy")
	}

	if err := d.Teardown(ctx, "integration-test"); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	out2, _, _ := (&ExecRunner{}).Run(ctx, "docker", "compose", "-p", "simpledeploy-integration-test", "ps", "--format", "json")
	if out2 != "" {
		t.Error("expected no containers after teardown")
	}
}
