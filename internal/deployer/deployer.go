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
	// Status is the post-stabilization outcome: "success", "unstable", or
	// "failed". "" when no stabilization ran (Skipped or composeErr present
	// before the check).
	Status   string
	Services []ServiceState
}

type ServiceStatus struct {
	Service string `json:"service"`
	State   string `json:"state"`
	Health  string `json:"health"`
}

// finishDeploy runs a stabilization check on a successful compose run and
// closes the deploy log with the resulting status. action is "deploy",
// "restart", or "pull"; on stabilization failure it becomes "<action>_unstable"
// and the WS done event carries status="unstable" + per-service detail.
// composeErr is the error from the preceding compose command (nil = success).
func (d *Deployer) finishDeploy(ctx context.Context, dl *DeployLog, slug, project, action string, composeErr error) (status string, services []ServiceState) {
	if composeErr != nil {
		d.Tracker.DoneWithLogStatus(slug, action+"_failed", "failed", nil)
		return "failed", nil
	}
	status, services = d.stabilize(ctx, dl, project)
	doneAction := action
	if status == "unstable" {
		doneAction = action + "_unstable"
	}
	d.Tracker.DoneWithLogStatus(slug, doneAction, status, services)
	return status, services
}

type Deployer struct {
	runner  CommandRunner
	Tracker *Tracker
	audit   AuditEmitter
}

// SetAuditEmitter wires in an AuditEmitter so Deploy/RollbackDeploy emit audit
// rows. Safe to call after construction; nil-safe at emit time.
func (d *Deployer) SetAuditEmitter(a AuditEmitter) { d.audit = a }

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

func (d *Deployer) Deploy(ctx context.Context, app *compose.AppConfig, auths ...RegistryAuth) (result DeployResult) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dl, fresh := d.Tracker.TrackWithLog(app.Name, cancel)
	if !fresh {
		// Another deploy for this slug is in flight; skip to avoid racing
		// docker compose on the same project and orphaning WS subscribers.
		return DeployResult{Skipped: true}
	}

	defer func() {
		if d.audit == nil || result.Skipped {
			return
		}
		evt := DeployAuditEvent{AppSlug: app.Name}
		if result.Err != nil {
			evt.Action = "deploy_failed"
			evt.Error = result.Err.Error()
		} else {
			evt.Action = "deploy_succeeded"
		}
		d.audit.RecordDeploy(ctx, evt)
	}()

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
	status, services := d.finishDeploy(ctx, dl, app.Name, project, "deploy", err)
	if err != nil {
		return DeployResult{Output: output, Err: fmt.Errorf("compose up: %w", err), Status: status, Services: services}
	}
	return DeployResult{Output: output, Status: status, Services: services}
}

// RollbackDeploy redeploys app from its current compose file (already written
// to disk by the caller) and emits a "rollback" audit event. The version and
// composeVersionID fields are set by the caller via opts.
func (d *Deployer) RollbackDeploy(ctx context.Context, app *compose.AppConfig, version int, composeVersionID *int64, auths ...RegistryAuth) (result DeployResult) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dl, fresh := d.Tracker.TrackWithLog(app.Name, cancel)
	if !fresh {
		return DeployResult{Skipped: true}
	}

	defer func() {
		if d.audit == nil || result.Skipped {
			return
		}
		evt := DeployAuditEvent{
			AppSlug:          app.Name,
			Action:           "rollback",
			Version:          version,
			ComposeVersionID: composeVersionID,
		}
		if result.Err != nil {
			evt.Error = result.Err.Error()
		}
		d.audit.RecordDeploy(ctx, evt)
	}()

	project := "simpledeploy-" + app.Name
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
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
	status, services := d.finishDeploy(ctx, dl, app.Name, project, "deploy", err)
	if err != nil {
		return DeployResult{Output: output, Err: fmt.Errorf("compose up: %w", err), Status: status, Services: services}
	}
	return DeployResult{Output: output, Status: status, Services: services}
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
	// Use `compose restart` (in-place restart of existing containers) rather
	// than `up -d --force-recreate`: it preserves container identity, skips
	// a pull/recreate/reattach cycle, and is what the UI "Restart" button
	// semantically means. Deploy/rollback paths still recreate.
	args := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"restart",
	}
	stdout, stderr, err := d.runCmd(ctx, dl, "docker", args...)
	output := strings.TrimSpace(stdout + "\n" + stderr)
	status, services := d.finishDeploy(ctx, dl, app.Name, project, "restart", err)
	if err != nil {
		return DeployResult{Output: output, Err: fmt.Errorf("compose restart: %w", err), Status: status, Services: services}
	}
	return DeployResult{Output: output, Status: status, Services: services}
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
		d.Tracker.DoneWithLogStatus(app.Name, "pull_failed", "failed", nil)
		return DeployResult{Output: output, Err: fmt.Errorf("compose pull: %w", err), Status: "failed"}
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
	status, services := d.finishDeploy(ctx, dl, app.Name, project, "pull", err)
	if err != nil {
		return DeployResult{Output: output, Err: fmt.Errorf("compose up after pull: %w", err), Status: status, Services: services}
	}
	return DeployResult{Output: output, Status: status, Services: services}
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
