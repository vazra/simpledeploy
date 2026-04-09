package deployer

import (
	"context"
	"fmt"

	"github.com/vazra/simpledeploy/internal/compose"
)

type Deployer struct {
	runner CommandRunner
}

func New(runner CommandRunner) (*Deployer, error) {
	d := &Deployer{runner: runner}
	_, stderr, err := d.runner.Run(context.Background(), "docker", "compose", "version")
	if err != nil {
		return nil, fmt.Errorf("docker compose not available: %s: %w", stderr, err)
	}
	return d, nil
}

func (d *Deployer) Deploy(ctx context.Context, app *compose.AppConfig) error {
	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose up: %s: %w", stderr, err)
	}
	return nil
}

func (d *Deployer) Teardown(ctx context.Context, projectName string) error {
	project := "simpledeploy-" + projectName
	args := []string{
		"compose",
		"-p", project,
		"down",
		"--remove-orphans",
	}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose down: %s: %w", stderr, err)
	}
	return nil
}
