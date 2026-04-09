package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
)

type ServiceStatus struct {
	Service string `json:"service"`
	State   string `json:"state"`
	Health  string `json:"health"`
}

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

func (d *Deployer) Status(ctx context.Context, projectName string) ([]ServiceStatus, error) {
	project := "simpledeploy-" + projectName
	stdout, stderr, err := d.runner.Run(ctx, "docker", "compose", "-p", project, "ps", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("compose ps: %s: %w", stderr, err)
	}
	if stdout == "" {
		return nil, nil
	}
	var raw []struct {
		Service string `json:"Service"`
		State   string `json:"State"`
		Health  string `json:"Health"`
	}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		raw = nil
		for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
			if line == "" {
				continue
			}
			var item struct {
				Service string `json:"Service"`
				State   string `json:"State"`
				Health  string `json:"Health"`
			}
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				continue
			}
			raw = append(raw, item)
		}
	}
	result := make([]ServiceStatus, len(raw))
	for i, r := range raw {
		result[i] = ServiceStatus{Service: r.Service, State: r.State, Health: r.Health}
	}
	return result, nil
}
