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

func (d *Deployer) Restart(ctx context.Context, app *compose.AppConfig) error {
	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--force-recreate",
		"--remove-orphans",
	}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose restart: %s: %w", stderr, err)
	}
	return nil
}

func (d *Deployer) Stop(ctx context.Context, projectName string) error {
	project := "simpledeploy-" + projectName
	args := []string{"compose", "-p", project, "stop"}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose stop: %s: %w", stderr, err)
	}
	return nil
}

func (d *Deployer) Start(ctx context.Context, projectName string) error {
	project := "simpledeploy-" + projectName
	args := []string{"compose", "-p", project, "start"}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose start: %s: %w", stderr, err)
	}
	return nil
}

func (d *Deployer) Pull(ctx context.Context, app *compose.AppConfig) error {
	project := "simpledeploy-" + app.Name
	pullArgs := []string{"compose", "-f", app.ComposePath, "-p", project, "pull"}
	_, stderr, err := d.runner.Run(ctx, "docker", pullArgs...)
	if err != nil {
		return fmt.Errorf("compose pull: %s: %w", stderr, err)
	}
	upArgs := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	_, stderr, err = d.runner.Run(ctx, "docker", upArgs...)
	if err != nil {
		return fmt.Errorf("compose up after pull: %s: %w", stderr, err)
	}
	return nil
}

func (d *Deployer) Scale(ctx context.Context, app *compose.AppConfig, scales map[string]int) error {
	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--no-recreate",
		"--remove-orphans",
	}
	for svc, n := range scales {
		args = append(args, "--scale", fmt.Sprintf("%s=%d", svc, n))
	}
	_, stderr, err := d.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("compose scale: %s: %w", stderr, err)
	}
	return nil
}
