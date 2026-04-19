package deployer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
)

type RegistryAuth struct {
	URL      string
	Username string
	Password string
}

type DeployResult struct {
	Output  string
	Err     error
	Skipped bool // true when another deploy for the same slug was already in flight
}

type ServiceStatus struct {
	Service string `json:"service"`
	State   string `json:"state"`
	Health  string `json:"health"`
}

type Deployer struct {
	runner  CommandRunner
	Tracker *Tracker
}

func New(runner CommandRunner) (*Deployer, error) {
	d := &Deployer{runner: runner, Tracker: NewTracker()}
	_, stderr, err := d.runner.Run(context.Background(), "docker", "compose", "version")
	if err != nil {
		return nil, fmt.Errorf("docker compose not available: %s: %w", stderr, err)
	}
	return d, nil
}

// runCmd uses RunStreaming when a DeployLog is available, otherwise falls back to d.runner.Run.
func (d *Deployer) runCmd(ctx context.Context, dl *DeployLog, name string, args ...string) (string, string, error) {
	if dl != nil {
		return RunStreaming(ctx, dl, name, args...)
	}
	return d.runner.Run(ctx, name, args...)
}

func (d *Deployer) Deploy(ctx context.Context, app *compose.AppConfig, auths ...RegistryAuth) DeployResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dl, fresh := d.Tracker.TrackWithLog(app.Name, cancel)
	if !fresh {
		// Another deploy for this slug is in flight; skip to avoid racing
		// docker compose on the same project and orphaning WS subscribers.
		return DeployResult{Skipped: true}
	}

	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	// Prepend --config <tmpDir> so `docker compose up` picks up registry auth.
	if len(auths) > 0 {
		tmpDir, err := writeDockerConfig(auths)
		if err != nil {
			d.Tracker.DoneWithLog(app.Name, "deploy_failed")
			return DeployResult{Err: fmt.Errorf("write docker config: %w", err)}
		}
		defer os.RemoveAll(tmpDir)
		args = append([]string{"--config", tmpDir}, args...)
	}
	stdout, stderr, err := d.runCmd(ctx, dl, "docker", args...)
	output := strings.TrimSpace(stdout + "\n" + stderr)
	action := "deploy"
	if err != nil {
		action = "deploy_failed"
		d.Tracker.DoneWithLog(app.Name, action)
		return DeployResult{Output: output, Err: fmt.Errorf("compose up: %w", err)}
	}
	d.Tracker.DoneWithLog(app.Name, action)
	return DeployResult{Output: output}
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

func (d *Deployer) Restart(ctx context.Context, app *compose.AppConfig) DeployResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dl, fresh := d.Tracker.TrackWithLog(app.Name, cancel)
	if !fresh {
		return DeployResult{Skipped: true}
	}

	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--force-recreate",
		"--remove-orphans",
	}
	stdout, stderr, err := d.runCmd(ctx, dl, "docker", args...)
	output := strings.TrimSpace(stdout + "\n" + stderr)
	action := "restart"
	if err != nil {
		action = "restart_failed"
		d.Tracker.DoneWithLog(app.Name, action)
		return DeployResult{Output: output, Err: fmt.Errorf("compose restart: %w", err)}
	}
	d.Tracker.DoneWithLog(app.Name, action)
	return DeployResult{Output: output}
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

func (d *Deployer) Pull(ctx context.Context, app *compose.AppConfig, auths []RegistryAuth) DeployResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dl, fresh := d.Tracker.TrackWithLog(app.Name, cancel)
	if !fresh {
		return DeployResult{Skipped: true}
	}

	project := "simpledeploy-" + app.Name

	pullArgs := []string{"compose", "-f", app.ComposePath, "-p", project, "pull"}

	if len(auths) > 0 {
		tmpDir, err := writeDockerConfig(auths)
		if err != nil {
			d.Tracker.DoneWithLog(app.Name, "pull_failed")
			return DeployResult{Err: fmt.Errorf("write docker config: %w", err)}
		}
		defer os.RemoveAll(tmpDir)
		pullArgs = []string{"--config", tmpDir, "compose", "-f", app.ComposePath, "-p", project, "pull"}
	}

	stdout, stderr, err := d.runCmd(ctx, dl, "docker", pullArgs...)
	output := strings.TrimSpace(stdout + "\n" + stderr)
	if err != nil {
		d.Tracker.DoneWithLog(app.Name, "pull_failed")
		return DeployResult{Output: output, Err: fmt.Errorf("compose pull: %w", err)}
	}

	upArgs := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	stdout, stderr, err = d.runCmd(ctx, dl, "docker", upArgs...)
	output += "\n" + strings.TrimSpace(stdout+"\n"+stderr)
	output = strings.TrimSpace(output)
	if err != nil {
		d.Tracker.DoneWithLog(app.Name, "pull_failed")
		return DeployResult{Output: output, Err: fmt.Errorf("compose up after pull: %w", err)}
	}
	d.Tracker.DoneWithLog(app.Name, "pull")
	return DeployResult{Output: output}
}

func writeDockerConfig(auths []RegistryAuth) (string, error) {
	type authEntry struct {
		Auth string `json:"auth"`
	}
	configData := struct {
		Auths map[string]authEntry `json:"auths"`
	}{
		Auths: make(map[string]authEntry, len(auths)),
	}
	for _, a := range auths {
		encoded := base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
		configData.Auths[a.URL] = authEntry{Auth: encoded}
	}
	data, err := json.Marshal(configData)
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp("", "simpledeploy-docker-*")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0600); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpDir, nil
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

func (d *Deployer) Cancel(ctx context.Context, app *compose.AppConfig) error {
	if err := d.Tracker.Cancel(app.Name); err != nil {
		return err
	}
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
		return fmt.Errorf("reconcile after cancel: %s: %w", stderr, err)
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
