# Phase 2: Reconciler & Compose - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drop a docker-compose.yml into a directory (or `simpledeploy apply`), and simpledeploy deploys it via Docker API. List apps and their status. Remove apps. Directory watcher for automatic reconciliation.

**Architecture:** Compose files parsed via compose-go library. A deployer translates parsed specs to Docker API calls (networks, images, containers). A reconciler diffs desired vs actual state and drives the deployer. fsnotify watches the apps directory for changes. CLI commands and API endpoints provide manual control.

**Tech Stack:** compose-go (compose parsing), Docker Go SDK (container lifecycle), fsnotify (directory watching), existing store/api/docker packages from Phase 1

---

## File Structure

```
internal/compose/parser.go              - Parse compose files, extract simpledeploy labels
internal/compose/parser_test.go          - Parser tests with fixture compose files
internal/compose/testdata/basic.yml      - Test fixture: basic compose file
internal/compose/testdata/multi.yml      - Test fixture: multi-service compose file

internal/docker/client.go               - Expand Client interface with container lifecycle methods
internal/docker/client_test.go           - Expand tests
internal/docker/mock.go                  - Mock client for testing

internal/store/apps.go                   - App CRUD operations
internal/store/apps_test.go              - App CRUD tests
internal/store/migrations/002_app_labels.sql  - app_labels table

internal/deployer/deployer.go           - Translate compose spec to Docker API calls
internal/deployer/deployer_test.go      - Deployer tests using mock Docker client

internal/reconciler/reconciler.go       - Diff desired vs actual, drive deployer
internal/reconciler/watcher.go          - fsnotify directory watcher
internal/reconciler/reconciler_test.go  - Reconciler tests

internal/api/server.go                  - Expand with store/docker deps, app endpoints
internal/api/apps.go                    - App API handlers
internal/api/apps_test.go               - App endpoint tests

cmd/simpledeploy/main.go               - Add apply/remove/list commands, wire reconciler
```

---

### Task 1: Compose Parser

**Files:**
- Create: `internal/compose/parser.go`
- Create: `internal/compose/parser_test.go`
- Create: `internal/compose/testdata/basic.yml`
- Create: `internal/compose/testdata/multi.yml`

- [ ] **Step 1: Add compose-go dependency**

```bash
cd /Users/ameen/dev/vazra/simpledeploy
go get github.com/docker/compose-go/v2@latest
```

- [ ] **Step 2: Create test fixtures**

Create `internal/compose/testdata/basic.yml`:

```yaml
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
    environment:
      - APP_ENV=production
    volumes:
      - ./data:/app/data
    labels:
      simpledeploy.domain: "myapp.example.com"
      simpledeploy.port: "80"
      simpledeploy.tls: "auto"
    restart: unless-stopped
```

Create `internal/compose/testdata/multi.yml`:

```yaml
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: "postgres://db:5432/myapp"
    labels:
      simpledeploy.domain: "myapp.example.com"
      simpledeploy.port: "3000"
    depends_on:
      - db
    restart: unless-stopped
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: myapp
      POSTGRES_PASSWORD: secret
    volumes:
      - pgdata:/var/lib/postgresql/data
    labels:
      simpledeploy.backup.strategy: "postgres"
      simpledeploy.backup.schedule: "0 2 * * *"
    restart: unless-stopped

volumes:
  pgdata:
```

- [ ] **Step 3: Write failing tests**

Create `internal/compose/parser_test.go`:

```go
package compose

import (
	"testing"
)

func TestParseBasicCompose(t *testing.T) {
	app, err := ParseFile("testdata/basic.yml", "myapp")
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}

	if app.Name != "myapp" {
		t.Errorf("Name = %q, want myapp", app.Name)
	}
	if app.Domain != "myapp.example.com" {
		t.Errorf("Domain = %q, want myapp.example.com", app.Domain)
	}
	if app.Port != "80" {
		t.Errorf("Port = %q, want 80", app.Port)
	}
	if app.TLS != "auto" {
		t.Errorf("TLS = %q, want auto", app.TLS)
	}
	if len(app.Services) != 1 {
		t.Fatalf("Services len = %d, want 1", len(app.Services))
	}
	svc := app.Services[0]
	if svc.Name != "web" {
		t.Errorf("Service.Name = %q, want web", svc.Name)
	}
	if svc.Image != "nginx:latest" {
		t.Errorf("Service.Image = %q, want nginx:latest", svc.Image)
	}
}

func TestParseMultiServiceCompose(t *testing.T) {
	app, err := ParseFile("testdata/multi.yml", "myapp")
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}

	if len(app.Services) != 2 {
		t.Fatalf("Services len = %d, want 2", len(app.Services))
	}
	if app.Domain != "myapp.example.com" {
		t.Errorf("Domain = %q, want myapp.example.com", app.Domain)
	}

	// check backup labels extracted from db service
	if app.BackupStrategy != "postgres" {
		t.Errorf("BackupStrategy = %q, want postgres", app.BackupStrategy)
	}
	if app.BackupSchedule != "0 2 * * *" {
		t.Errorf("BackupSchedule = %q, want '0 2 * * *'", app.BackupSchedule)
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := ParseFile("testdata/nonexistent.yml", "test")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExtractLabels(t *testing.T) {
	labels := map[string]string{
		"simpledeploy.domain":           "app.example.com",
		"simpledeploy.port":             "8080",
		"simpledeploy.tls":              "off",
		"simpledeploy.backup.strategy":  "volume",
		"simpledeploy.backup.schedule":  "0 3 * * *",
		"simpledeploy.backup.target":    "s3",
		"simpledeploy.backup.retention": "5",
		"simpledeploy.alerts.cpu":       ">90,10m",
		"simpledeploy.alerts.memory":    ">95,5m",
		"simpledeploy.ratelimit.requests": "50",
		"simpledeploy.ratelimit.window":   "30s",
		"simpledeploy.ratelimit.by":       "ip",
		"simpledeploy.ratelimit.burst":    "10",
		"simpledeploy.path.patterns":      "/users/{id},/posts/{id}",
		"unrelated.label":               "ignored",
	}

	cfg := ExtractLabels(labels)

	if cfg.Domain != "app.example.com" {
		t.Errorf("Domain = %q, want app.example.com", cfg.Domain)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.TLS != "off" {
		t.Errorf("TLS = %q, want off", cfg.TLS)
	}
	if cfg.BackupStrategy != "volume" {
		t.Errorf("BackupStrategy = %q, want volume", cfg.BackupStrategy)
	}
	if cfg.RateLimit.Requests != "50" {
		t.Errorf("RateLimit.Requests = %q, want 50", cfg.RateLimit.Requests)
	}
	if cfg.PathPatterns != "/users/{id},/posts/{id}" {
		t.Errorf("PathPatterns = %q, want /users/{id},/posts/{id}", cfg.PathPatterns)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/compose/ -v`
Expected: FAIL - package doesn't exist

- [ ] **Step 5: Implement parser**

Create `internal/compose/parser.go`:

```go
package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/compose-go/v2/loader"
	"github.com/docker/compose-go/v2/types"
)

type AppConfig struct {
	Name           string
	ComposePath    string
	Domain         string
	Port           string
	TLS            string
	BackupStrategy string
	BackupSchedule string
	BackupTarget   string
	BackupRetention string
	AlertCPU       string
	AlertMemory    string
	PathPatterns   string
	RateLimit      RateLimitLabels
	Services       []ServiceConfig
	Project        *types.Project
}

type RateLimitLabels struct {
	Requests string
	Window   string
	By       string
	Burst    string
}

type ServiceConfig struct {
	Name        string
	Image       string
	Ports       []PortMapping
	Environment map[string]string
	Volumes     []VolumeMount
	Restart     string
	Labels      map[string]string
	DependsOn   []string
}

type PortMapping struct {
	Host      string
	Container string
	Protocol  string
}

type VolumeMount struct {
	Source   string
	Target  string
	Type    string // "bind" or "volume"
}

func ParseFile(path string, appName string) (*AppConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}

	project, err := loader.LoadWithContext(context.Background(), types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{
			{Filename: absPath, Content: data},
		},
		WorkingDir: filepath.Dir(absPath),
	}, func(o *loader.Options) {
		o.SetProjectName(appName, true)
		o.SkipValidation = true
	})
	if err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	app := &AppConfig{
		Name:        appName,
		ComposePath: absPath,
		Project:     project,
	}

	// extract services and collect all simpledeploy labels
	allLabels := make(map[string]string)
	for name, svc := range project.Services {
		sc := ServiceConfig{
			Name:        name,
			Image:       svc.Image,
			Environment: make(map[string]string),
			Labels:      svc.Labels,
			Restart:     svc.Restart,
		}

		// ports
		for _, p := range svc.Ports {
			sc.Ports = append(sc.Ports, PortMapping{
				Host:      fmt.Sprintf("%d", p.Published),
				Container: fmt.Sprintf("%d", p.Target),
				Protocol:  p.Protocol,
			})
		}

		// environment
		for k, v := range svc.Environment {
			if v != nil {
				sc.Environment[k] = *v
			}
		}

		// volumes
		for _, v := range svc.Volumes {
			vm := VolumeMount{
				Source: v.Source,
				Target: v.Target,
				Type:   string(v.Type),
			}
			sc.Volumes = append(sc.Volumes, vm)
		}

		// depends_on
		for dep := range svc.DependsOn {
			sc.DependsOn = append(sc.DependsOn, dep)
		}

		app.Services = append(app.Services, sc)

		// merge simpledeploy labels
		for k, v := range svc.Labels {
			if strings.HasPrefix(k, "simpledeploy.") {
				allLabels[k] = v
			}
		}
	}

	// apply extracted labels
	extracted := ExtractLabels(allLabels)
	app.Domain = extracted.Domain
	app.Port = extracted.Port
	app.TLS = extracted.TLS
	app.BackupStrategy = extracted.BackupStrategy
	app.BackupSchedule = extracted.BackupSchedule
	app.BackupTarget = extracted.BackupTarget
	app.BackupRetention = extracted.BackupRetention
	app.AlertCPU = extracted.AlertCPU
	app.AlertMemory = extracted.AlertMemory
	app.PathPatterns = extracted.PathPatterns
	app.RateLimit = extracted.RateLimit

	return app, nil
}

type LabelConfig struct {
	Domain          string
	Port            string
	TLS             string
	BackupStrategy  string
	BackupSchedule  string
	BackupTarget    string
	BackupRetention string
	AlertCPU        string
	AlertMemory     string
	PathPatterns    string
	RateLimit       RateLimitLabels
}

func ExtractLabels(labels map[string]string) LabelConfig {
	cfg := LabelConfig{}
	for k, v := range labels {
		switch k {
		case "simpledeploy.domain":
			cfg.Domain = v
		case "simpledeploy.port":
			cfg.Port = v
		case "simpledeploy.tls":
			cfg.TLS = v
		case "simpledeploy.backup.strategy":
			cfg.BackupStrategy = v
		case "simpledeploy.backup.schedule":
			cfg.BackupSchedule = v
		case "simpledeploy.backup.target":
			cfg.BackupTarget = v
		case "simpledeploy.backup.retention":
			cfg.BackupRetention = v
		case "simpledeploy.alerts.cpu":
			cfg.AlertCPU = v
		case "simpledeploy.alerts.memory":
			cfg.AlertMemory = v
		case "simpledeploy.path.patterns":
			cfg.PathPatterns = v
		case "simpledeploy.ratelimit.requests":
			cfg.RateLimit.Requests = v
		case "simpledeploy.ratelimit.window":
			cfg.RateLimit.Window = v
		case "simpledeploy.ratelimit.by":
			cfg.RateLimit.By = v
		case "simpledeploy.ratelimit.burst":
			cfg.RateLimit.Burst = v
		}
	}
	return cfg
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/compose/ -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/compose/ go.mod go.sum
git commit -m "add compose file parser with label extraction"
```

---

### Task 2: Expand Docker Client for Container Lifecycle

**Files:**
- Modify: `internal/docker/client.go`
- Modify: `internal/docker/client_test.go`
- Create: `internal/docker/mock.go`

- [ ] **Step 1: Write mock client and expand interface**

Create `internal/docker/mock.go`:

```go
package docker

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
)

type MockClient struct {
	mu         sync.Mutex
	Calls      []string
	Containers map[string]*container.InspectResponse
	Networks   map[string]bool
	PullErr    error
	CreateErr  error
	StartErr   error
	StopErr    error
	RemoveErr  error
}

func NewMockClient() *MockClient {
	return &MockClient{
		Containers: make(map[string]*container.InspectResponse),
		Networks:   make(map[string]bool),
	}
}

func (m *MockClient) Ping(ctx context.Context) error {
	m.record("Ping")
	return nil
}

func (m *MockClient) Close() error {
	return nil
}

func (m *MockClient) NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	m.record("NetworkCreate:" + name)
	m.mu.Lock()
	m.Networks[name] = true
	m.mu.Unlock()
	return network.CreateResponse{ID: "net-" + name}, nil
}

func (m *MockClient) NetworkRemove(ctx context.Context, name string) error {
	m.record("NetworkRemove:" + name)
	m.mu.Lock()
	delete(m.Networks, name)
	m.mu.Unlock()
	return nil
}

func (m *MockClient) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	m.record("ImagePull:" + ref)
	if m.PullErr != nil {
		return nil, m.PullErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, name string) (container.CreateResponse, error) {
	m.record("ContainerCreate:" + name)
	if m.CreateErr != nil {
		return container.CreateResponse{}, m.CreateErr
	}
	id := "ctr-" + name
	m.mu.Lock()
	m.Containers[id] = &container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   id,
			Name: "/" + name,
		},
	}
	m.mu.Unlock()
	return container.CreateResponse{ID: id}, nil
}

func (m *MockClient) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	m.record("ContainerStart:" + id)
	return m.StartErr
}

func (m *MockClient) ContainerStop(ctx context.Context, id string, opts container.StopOptions) error {
	m.record("ContainerStop:" + id)
	return m.StopErr
}

func (m *MockClient) ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	m.record("ContainerRemove:" + id)
	m.mu.Lock()
	delete(m.Containers, id)
	m.mu.Unlock()
	return m.RemoveErr
}

func (m *MockClient) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	m.record("ContainerList")
	var result []container.Summary
	m.mu.Lock()
	for id, c := range m.Containers {
		summary := container.Summary{
			ID:    id,
			Names: []string{c.Name},
		}
		result = append(result, summary)
	}
	m.mu.Unlock()
	return result, nil
}

func (m *MockClient) record(call string) {
	m.mu.Lock()
	m.Calls = append(m.Calls, call)
	m.mu.Unlock()
}

func (m *MockClient) HasCall(prefix string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.Calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Expand Client interface in client.go**

Update `internal/docker/client.go` to expand the interface and implement new methods:

```go
package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
)

type Client interface {
	Ping(ctx context.Context) error
	Close() error
	NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(ctx context.Context, name string) error
	ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, name string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, id string, opts container.StartOptions) error
	ContainerStop(ctx context.Context, id string, opts container.StopOptions) error
	ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error
	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
}

type DockerClient struct {
	cli *dockerclient.Client
}

func NewClient() (*DockerClient, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &DockerClient{cli: cli}, nil
}

func (c *DockerClient) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

func (c *DockerClient) Close() error {
	return c.cli.Close()
}

func (c *DockerClient) NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	return c.cli.NetworkCreate(ctx, name, opts)
}

func (c *DockerClient) NetworkRemove(ctx context.Context, name string) error {
	return c.cli.NetworkRemove(ctx, name)
}

func (c *DockerClient) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	return c.cli.ImagePull(ctx, ref, opts)
}

func (c *DockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, name string) (container.CreateResponse, error) {
	return c.cli.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, name)
}

func (c *DockerClient) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	return c.cli.ContainerStart(ctx, id, opts)
}

func (c *DockerClient) ContainerStop(ctx context.Context, id string, opts container.StopOptions) error {
	return c.cli.ContainerStop(ctx, id, opts)
}

func (c *DockerClient) ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	return c.cli.ContainerRemove(ctx, id, opts)
}

func (c *DockerClient) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	return c.cli.ContainerList(ctx, opts)
}

func (c *DockerClient) Raw() *dockerclient.Client {
	return c.cli
}
```

- [ ] **Step 3: Update tests for interface conformance**

Update `internal/docker/client_test.go`:

```go
package docker

import (
	"context"
	"testing"
)

var _ Client = (*DockerClient)(nil)
var _ Client = (*MockClient)(nil)

func TestNewClientDoesNotPanic(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestPingWithDocker(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer c.Close()
	if err := c.Ping(context.Background()); err != nil {
		t.Skipf("Docker daemon not responding: %v", err)
	}
}

func TestMockClientRecordsCalls(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	m.Ping(ctx)
	if !m.HasCall("Ping") {
		t.Error("expected Ping call recorded")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/docker/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/docker/
git commit -m "expand Docker client with container lifecycle and mock"
```

---

### Task 3: Store - App CRUD and Labels

**Files:**
- Create: `internal/store/apps.go`
- Create: `internal/store/apps_test.go`
- Create: `internal/store/migrations/002_app_labels.sql`

- [ ] **Step 1: Create migration**

Create `internal/store/migrations/002_app_labels.sql`:

```sql
CREATE TABLE IF NOT EXISTS app_labels (
    app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (app_id, key)
);

CREATE INDEX IF NOT EXISTS idx_app_labels_app ON app_labels(app_id);
```

- [ ] **Step 2: Write failing tests**

Create `internal/store/apps_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndGetApp(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "my-app",
		Slug:        "my-app",
		ComposePath: "/etc/simpledeploy/apps/my-app/docker-compose.yml",
		Status:      "running",
		Domain:      "myapp.example.com",
	}
	labels := map[string]string{
		"simpledeploy.domain": "myapp.example.com",
		"simpledeploy.port":   "3000",
	}

	err := s.UpsertApp(app, labels)
	if err != nil {
		t.Fatalf("UpsertApp() error: %v", err)
	}
	if app.ID == 0 {
		t.Error("expected app.ID to be set after insert")
	}

	got, err := s.GetAppBySlug("my-app")
	if err != nil {
		t.Fatalf("GetAppBySlug() error: %v", err)
	}
	if got.Name != "my-app" {
		t.Errorf("Name = %q, want my-app", got.Name)
	}
	if got.Domain != "myapp.example.com" {
		t.Errorf("Domain = %q, want myapp.example.com", got.Domain)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
}

func TestUpsertAppUpdate(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name: "my-app", Slug: "my-app",
		ComposePath: "/tmp/compose.yml", Status: "stopped",
	}
	s.UpsertApp(app, nil)
	origID := app.ID

	app.Status = "running"
	app.Domain = "new.example.com"
	s.UpsertApp(app, map[string]string{"simpledeploy.domain": "new.example.com"})

	got, _ := s.GetAppBySlug("my-app")
	if got.ID != origID {
		t.Errorf("expected same ID after update, got %d want %d", got.ID, origID)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.Domain != "new.example.com" {
		t.Errorf("Domain = %q, want new.example.com", got.Domain)
	}
}

func TestListApps(t *testing.T) {
	s := newTestStore(t)

	s.UpsertApp(&App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	s.UpsertApp(&App{Name: "app2", Slug: "app2", ComposePath: "/tmp/2.yml", Status: "stopped"}, nil)

	apps, err := s.ListApps()
	if err != nil {
		t.Fatalf("ListApps() error: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("ListApps() len = %d, want 2", len(apps))
	}
}

func TestDeleteApp(t *testing.T) {
	s := newTestStore(t)

	s.UpsertApp(&App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "running"}, nil)
	err := s.DeleteApp("app1")
	if err != nil {
		t.Fatalf("DeleteApp() error: %v", err)
	}

	_, err = s.GetAppBySlug("app1")
	if err == nil {
		t.Error("expected error after deleting app")
	}
}

func TestGetAppBySlugNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetAppBySlug("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent app")
	}
}

func TestGetAppLabels(t *testing.T) {
	s := newTestStore(t)

	labels := map[string]string{
		"simpledeploy.domain": "test.example.com",
		"simpledeploy.port":   "8080",
	}
	s.UpsertApp(&App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "running"}, labels)

	got, err := s.GetAppLabels("app1")
	if err != nil {
		t.Fatalf("GetAppLabels() error: %v", err)
	}
	if got["simpledeploy.domain"] != "test.example.com" {
		t.Errorf("domain label = %q, want test.example.com", got["simpledeploy.domain"])
	}
	if got["simpledeploy.port"] != "8080" {
		t.Errorf("port label = %q, want 8080", got["simpledeploy.port"])
	}
}

func TestUpdateAppStatus(t *testing.T) {
	s := newTestStore(t)

	s.UpsertApp(&App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "stopped"}, nil)
	err := s.UpdateAppStatus("app1", "running")
	if err != nil {
		t.Fatalf("UpdateAppStatus() error: %v", err)
	}

	got, _ := s.GetAppBySlug("app1")
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/store/ -v -run TestUpsert`
Expected: FAIL - App type and methods don't exist

- [ ] **Step 4: Implement app CRUD**

Create `internal/store/apps.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type App struct {
	ID          int64
	Name        string
	Slug        string
	ComposePath string
	Status      string
	Domain      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (s *Store) UpsertApp(app *App, labels map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	var existingID int64
	err = tx.QueryRow("SELECT id FROM apps WHERE slug = ?", app.Slug).Scan(&existingID)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(
			`INSERT INTO apps (name, slug, compose_path, status, domain) VALUES (?, ?, ?, ?, ?)`,
			app.Name, app.Slug, app.ComposePath, app.Status, app.Domain,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("insert app: %w", err)
		}
		app.ID, _ = res.LastInsertId()
	} else if err != nil {
		tx.Rollback()
		return fmt.Errorf("check existing app: %w", err)
	} else {
		app.ID = existingID
		_, err = tx.Exec(
			`UPDATE apps SET name=?, compose_path=?, status=?, domain=?, updated_at=datetime('now') WHERE id=?`,
			app.Name, app.ComposePath, app.Status, app.Domain, app.ID,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("update app: %w", err)
		}
	}

	// replace labels
	if labels != nil {
		if _, err := tx.Exec("DELETE FROM app_labels WHERE app_id = ?", app.ID); err != nil {
			tx.Rollback()
			return fmt.Errorf("delete old labels: %w", err)
		}
		for k, v := range labels {
			if _, err := tx.Exec("INSERT INTO app_labels (app_id, key, value) VALUES (?, ?, ?)", app.ID, k, v); err != nil {
				tx.Rollback()
				return fmt.Errorf("insert label: %w", err)
			}
		}
	}

	return tx.Commit()
}

func (s *Store) GetAppBySlug(slug string) (*App, error) {
	app := &App{}
	var domain sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, slug, compose_path, status, domain, created_at, updated_at FROM apps WHERE slug = ?`,
		slug,
	).Scan(&app.ID, &app.Name, &app.Slug, &app.ComposePath, &app.Status, &domain, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get app %q: %w", slug, err)
	}
	app.Domain = domain.String
	return app, nil
}

func (s *Store) ListApps() ([]App, error) {
	rows, err := s.db.Query(
		`SELECT id, name, slug, compose_path, status, domain, created_at, updated_at FROM apps ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var app App
		var domain sql.NullString
		if err := rows.Scan(&app.ID, &app.Name, &app.Slug, &app.ComposePath, &app.Status, &domain, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		app.Domain = domain.String
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (s *Store) DeleteApp(slug string) error {
	res, err := s.db.Exec("DELETE FROM apps WHERE slug = ?", slug)
	if err != nil {
		return fmt.Errorf("delete app %q: %w", slug, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("app %q not found", slug)
	}
	return nil
}

func (s *Store) UpdateAppStatus(slug, status string) error {
	_, err := s.db.Exec(
		`UPDATE apps SET status=?, updated_at=datetime('now') WHERE slug=?`,
		status, slug,
	)
	return err
}

func (s *Store) GetAppLabels(slug string) (map[string]string, error) {
	rows, err := s.db.Query(
		`SELECT al.key, al.value FROM app_labels al JOIN apps a ON al.app_id = a.id WHERE a.slug = ?`,
		slug,
	)
	if err != nil {
		return nil, fmt.Errorf("get labels for %q: %w", slug, err)
	}
	defer rows.Close()

	labels := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels[k] = v
	}
	return labels, rows.Err()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: all tests PASS (original 3 + new 7)

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "add app CRUD operations and labels table"
```

---

### Task 4: Deployer

**Files:**
- Create: `internal/deployer/deployer.go`
- Create: `internal/deployer/deployer_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/deployer/deployer_test.go`:

```go
package deployer

import (
	"context"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/docker"
)

func TestDeployCreatesNetworkAndContainers(t *testing.T) {
	mock := docker.NewMockClient()
	d := New(mock)

	app := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{
				Name:  "web",
				Image: "nginx:latest",
				Ports: []compose.PortMapping{
					{Host: "8080", Container: "80"},
				},
				Environment: map[string]string{"APP_ENV": "test"},
				Restart:     "unless-stopped",
			},
		},
	}

	err := d.Deploy(context.Background(), app)
	if err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	if !mock.HasCall("NetworkCreate:simpledeploy-testapp") {
		t.Error("expected NetworkCreate call for project network")
	}
	if !mock.HasCall("ImagePull:nginx:latest") {
		t.Error("expected ImagePull call for nginx:latest")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-testapp-web") {
		t.Error("expected ContainerCreate call")
	}
	if !mock.HasCall("ContainerStart:ctr-simpledeploy-testapp-web") {
		t.Error("expected ContainerStart call")
	}
}

func TestDeployMultipleServices(t *testing.T) {
	mock := docker.NewMockClient()
	d := New(mock)

	app := &compose.AppConfig{
		Name: "myapp",
		Services: []compose.ServiceConfig{
			{Name: "web", Image: "myapp:latest"},
			{Name: "db", Image: "postgres:16"},
		},
	}

	err := d.Deploy(context.Background(), app)
	if err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	if !mock.HasCall("ContainerCreate:simpledeploy-myapp-web") {
		t.Error("expected web container created")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-myapp-db") {
		t.Error("expected db container created")
	}
}

func TestTeardownRemovesContainersAndNetwork(t *testing.T) {
	mock := docker.NewMockClient()
	d := New(mock)

	// deploy first
	app := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{Name: "web", Image: "nginx:latest"},
		},
	}
	d.Deploy(context.Background(), app)

	// teardown
	err := d.Teardown(context.Background(), "testapp")
	if err != nil {
		t.Fatalf("Teardown() error: %v", err)
	}

	if !mock.HasCall("ContainerStop") {
		t.Error("expected ContainerStop call")
	}
	if !mock.HasCall("ContainerRemove") {
		t.Error("expected ContainerRemove call")
	}
	if !mock.HasCall("NetworkRemove:simpledeploy-testapp") {
		t.Error("expected NetworkRemove call")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/deployer/ -v`
Expected: FAIL - package doesn't exist

- [ ] **Step 3: Implement deployer**

Create `internal/deployer/deployer.go`:

```go
package deployer

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockernat "github.com/docker/go-connections/nat"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/docker"
)

const projectLabel = "simpledeploy.project"

type Deployer struct {
	docker docker.Client
}

func New(docker docker.Client) *Deployer {
	return &Deployer{docker: docker}
}

func (d *Deployer) Deploy(ctx context.Context, app *compose.AppConfig) error {
	networkName := fmt.Sprintf("simpledeploy-%s", app.Name)

	// create project network
	if _, err := d.docker.NetworkCreate(ctx, networkName, network.CreateOptions{
		Labels: map[string]string{projectLabel: app.Name},
	}); err != nil {
		return fmt.Errorf("create network %s: %w", networkName, err)
	}

	// deploy each service
	for _, svc := range app.Services {
		if err := d.deployService(ctx, app.Name, networkName, svc); err != nil {
			return fmt.Errorf("deploy service %s: %w", svc.Name, err)
		}
	}

	return nil
}

func (d *Deployer) deployService(ctx context.Context, projectName, networkName string, svc compose.ServiceConfig) error {
	// pull image
	reader, err := d.docker.ImagePull(ctx, svc.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", svc.Image, err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	containerName := fmt.Sprintf("simpledeploy-%s-%s", projectName, svc.Name)

	// build container config
	env := make([]string, 0, len(svc.Environment))
	for k, v := range svc.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	labels := map[string]string{
		projectLabel:          projectName,
		"simpledeploy.service": svc.Name,
	}
	for k, v := range svc.Labels {
		labels[k] = v
	}

	containerConfig := &container.Config{
		Image:  svc.Image,
		Env:    env,
		Labels: labels,
	}

	// port bindings
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for _, p := range svc.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		containerPort := nat.Port(fmt.Sprintf("%s/%s", p.Container, proto))
		exposedPorts[containerPort] = struct{}{}
		portBindings[containerPort] = []nat.PortBinding{
			{HostPort: p.Host},
		}
	}
	containerConfig.ExposedPorts = exposedPorts

	// host config
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: container.RestartPolicy{
			Name: restartPolicyName(svc.Restart),
		},
	}

	// volume mounts
	for _, v := range svc.Volumes {
		hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s", v.Source, v.Target))
	}

	// network config
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {},
		},
	}

	// create and start container
	resp, err := d.docker.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, containerName)
	if err != nil {
		return fmt.Errorf("create container %s: %w", containerName, err)
	}

	if err := d.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container %s: %w", containerName, err)
	}

	return nil
}

func (d *Deployer) Teardown(ctx context.Context, projectName string) error {
	networkName := fmt.Sprintf("simpledeploy-%s", projectName)

	// list containers with project label
	containers, err := d.docker.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", projectLabel, projectName)),
		),
	})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	// stop and remove each container
	for _, c := range containers {
		if err := d.docker.ContainerStop(ctx, c.ID, container.StopOptions{}); err != nil {
			return fmt.Errorf("stop container %s: %w", c.ID, err)
		}
		if err := d.docker.ContainerRemove(ctx, c.ID, container.RemoveOptions{}); err != nil {
			return fmt.Errorf("remove container %s: %w", c.ID, err)
		}
	}

	// remove network
	if err := d.docker.NetworkRemove(ctx, networkName); err != nil {
		return fmt.Errorf("remove network %s: %w", networkName, err)
	}

	return nil
}

func restartPolicyName(restart string) container.RestartPolicyMode {
	switch restart {
	case "always":
		return container.RestartPolicyAlways
	case "unless-stopped":
		return container.RestartPolicyUnlessStopped
	case "on-failure":
		return container.RestartPolicyOnFailure
	default:
		return container.RestartPolicyNo
	}
}
```

Note: The import for `nat` should be `dockernat "github.com/docker/go-connections/nat"` - use the alias `nat` in the code above. The actual import path is `github.com/docker/go-connections/nat`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/deployer/ -v`
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/deployer/
git commit -m "add deployer to translate compose specs to Docker API calls"
```

---

### Task 5: Reconciler + Directory Watcher

**Files:**
- Create: `internal/reconciler/reconciler.go`
- Create: `internal/reconciler/watcher.go`
- Create: `internal/reconciler/reconciler_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/reconciler/reconciler_test.go`:

```go
package reconciler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/store"
)

func newTestEnv(t *testing.T) (*Reconciler, *docker.MockClient, *store.Store, string) {
	t.Helper()

	dbDir := t.TempDir()
	s, err := store.Open(filepath.Join(dbDir, "test.db"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	mock := docker.NewMockClient()
	dep := deployer.New(mock)
	appsDir := t.TempDir()

	r := New(s, dep, appsDir)
	return r, mock, s, appsDir
}

func writeComposeFile(t *testing.T, dir, appName string) {
	t.Helper()
	appDir := filepath.Join(dir, appName)
	os.MkdirAll(appDir, 0755)
	content := `services:
  web:
    image: nginx:latest
    labels:
      simpledeploy.domain: "` + appName + `.example.com"
      simpledeploy.port: "80"
`
	os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte(content), 0644)
}

func TestReconcileNewApp(t *testing.T) {
	r, mock, s, appsDir := newTestEnv(t)

	writeComposeFile(t, appsDir, "myapp")

	err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}

	// verify deploy was called
	if !mock.HasCall("NetworkCreate:simpledeploy-myapp") {
		t.Error("expected network created for myapp")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-myapp-web") {
		t.Error("expected container created for myapp")
	}

	// verify app recorded in store
	app, err := s.GetAppBySlug("myapp")
	if err != nil {
		t.Fatalf("GetAppBySlug() error: %v", err)
	}
	if app.Status != "running" {
		t.Errorf("app status = %q, want running", app.Status)
	}
	if app.Domain != "myapp.example.com" {
		t.Errorf("app domain = %q, want myapp.example.com", app.Domain)
	}
}

func TestReconcileRemoveApp(t *testing.T) {
	r, mock, s, appsDir := newTestEnv(t)

	// first reconcile with app present
	writeComposeFile(t, appsDir, "myapp")
	r.Reconcile(context.Background())

	// remove the app directory
	os.RemoveAll(filepath.Join(appsDir, "myapp"))

	// reconcile again
	err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}

	if !mock.HasCall("NetworkRemove:simpledeploy-myapp") {
		t.Error("expected network removed for myapp")
	}

	_, err = s.GetAppBySlug("myapp")
	if err == nil {
		t.Error("expected app to be deleted from store")
	}
}

func TestReconcileMultipleApps(t *testing.T) {
	r, _, s, appsDir := newTestEnv(t)

	writeComposeFile(t, appsDir, "app1")
	writeComposeFile(t, appsDir, "app2")

	err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}

	apps, _ := s.ListApps()
	if len(apps) != 2 {
		t.Errorf("ListApps() len = %d, want 2", len(apps))
	}
}

func TestWatcherTriggersReconcile(t *testing.T) {
	r, mock, _, appsDir := newTestEnv(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Watch(ctx)
	}()

	// give watcher time to start
	time.Sleep(200 * time.Millisecond)

	// create an app
	writeComposeFile(t, appsDir, "watched-app")

	// wait for reconcile to trigger
	time.Sleep(2 * time.Second)

	cancel()

	if !mock.HasCall("ContainerCreate:simpledeploy-watched-app-web") {
		t.Error("expected watcher to trigger deploy for watched-app")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/reconciler/ -v`
Expected: FAIL - package doesn't exist

- [ ] **Step 3: Implement reconciler**

Create `internal/reconciler/reconciler.go`:

```go
package reconciler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

type Reconciler struct {
	store    *store.Store
	deployer *deployer.Deployer
	appsDir  string
}

func New(store *store.Store, deployer *deployer.Deployer, appsDir string) *Reconciler {
	return &Reconciler{
		store:    store,
		deployer: deployer,
		appsDir:  appsDir,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
	// scan apps directory for desired state
	desired, err := r.scanAppsDir()
	if err != nil {
		return fmt.Errorf("scan apps dir: %w", err)
	}

	// get current state from store
	current, err := r.store.ListApps()
	if err != nil {
		return fmt.Errorf("list current apps: %w", err)
	}

	currentMap := make(map[string]store.App)
	for _, app := range current {
		currentMap[app.Slug] = app
	}

	// deploy new or changed apps
	for slug, appConfig := range desired {
		if err := r.deployApp(ctx, slug, appConfig); err != nil {
			fmt.Fprintf(os.Stderr, "error deploying %s: %v\n", slug, err)
			continue
		}
		delete(currentMap, slug)
	}

	// remove apps no longer in directory
	for slug := range currentMap {
		if err := r.removeApp(ctx, slug); err != nil {
			fmt.Fprintf(os.Stderr, "error removing %s: %v\n", slug, err)
		}
	}

	return nil
}

func (r *Reconciler) scanAppsDir() (map[string]*compose.AppConfig, error) {
	entries, err := os.ReadDir(r.appsDir)
	if err != nil {
		return nil, fmt.Errorf("read apps dir: %w", err)
	}

	apps := make(map[string]*compose.AppConfig)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		if strings.HasPrefix(slug, ".") {
			continue
		}

		composePath := filepath.Join(r.appsDir, slug, "docker-compose.yml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			continue
		}

		appConfig, err := compose.ParseFile(composePath, slug)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", composePath, err)
			continue
		}

		apps[slug] = appConfig
	}

	return apps, nil
}

func (r *Reconciler) deployApp(ctx context.Context, slug string, appConfig *compose.AppConfig) error {
	if err := r.deployer.Deploy(ctx, appConfig); err != nil {
		r.store.UpsertApp(&store.App{
			Name: slug, Slug: slug,
			ComposePath: appConfig.ComposePath,
			Status: "error", Domain: appConfig.Domain,
		}, nil)
		return fmt.Errorf("deploy: %w", err)
	}

	// collect all simpledeploy labels
	labels := make(map[string]string)
	for _, svc := range appConfig.Services {
		for k, v := range svc.Labels {
			if strings.HasPrefix(k, "simpledeploy.") {
				labels[k] = v
			}
		}
	}

	return r.store.UpsertApp(&store.App{
		Name: slug, Slug: slug,
		ComposePath: appConfig.ComposePath,
		Status: "running", Domain: appConfig.Domain,
	}, labels)
}

func (r *Reconciler) removeApp(ctx context.Context, slug string) error {
	if err := r.deployer.Teardown(ctx, slug); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}
	return r.store.DeleteApp(slug)
}

func (r *Reconciler) DeployOne(ctx context.Context, composePath string, appName string) error {
	appConfig, err := compose.ParseFile(composePath, appName)
	if err != nil {
		return fmt.Errorf("parse compose file: %w", err)
	}
	return r.deployApp(ctx, appName, appConfig)
}

func (r *Reconciler) RemoveOne(ctx context.Context, appName string) error {
	return r.removeApp(ctx, appName)
}
```

- [ ] **Step 4: Implement watcher**

Create `internal/reconciler/watcher.go`:

```go
package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (r *Reconciler) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(r.appsDir); err != nil {
		return fmt.Errorf("watch %s: %w", r.appsDir, err)
	}

	// initial reconcile
	if err := r.Reconcile(ctx); err != nil {
		fmt.Printf("initial reconcile error: %v\n", err)
	}

	// debounce timer
	var debounce *time.Timer
	debounceDuration := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			_ = event
			// reset debounce timer on each event
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(debounceDuration, func() {
				if err := r.Reconcile(ctx); err != nil {
					fmt.Printf("reconcile error: %v\n", err)
				}
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("watcher error: %v\n", err)
		}
	}
}
```

- [ ] **Step 5: Add fsnotify dependency**

```bash
go get github.com/fsnotify/fsnotify@latest
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/reconciler/ -v -timeout 30s`
Expected: all 4 tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/reconciler/ go.mod go.sum
git commit -m "add reconciler with directory watcher and debounce"
```

---

### Task 6: CLI Commands (apply, remove, list)

**Files:**
- Modify: `cmd/simpledeploy/main.go`

- [ ] **Step 1: Add apply, remove, list commands**

Add the following commands to `cmd/simpledeploy/main.go`. Keep existing `serveCmd` and `initCmd`. Add these new commands and update `runServe` to start the reconciler watcher:

New commands to add to `init()`:

```go
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Deploy an app from a compose file",
	RunE:  runApply,
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a deployed app",
	RunE:  runRemove,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployed apps",
	RunE:  runList,
}
```

Register in `init()`:
```go
applyCmd.Flags().StringP("file", "f", "", "compose file path")
applyCmd.Flags().StringP("dir", "d", "", "directory of app subdirectories")
applyCmd.Flags().String("name", "", "app name (required with -f)")
removeCmd.Flags().String("name", "", "app name to remove")
removeCmd.MarkFlagRequired("name")
rootCmd.AddCommand(serveCmd, initCmd, applyCmd, removeCmd, listCmd)
```

Implement handlers:

```go
func runApply(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	file, _ := cmd.Flags().GetString("file")
	dir, _ := cmd.Flags().GetString("dir")
	name, _ := cmd.Flags().GetString("name")

	if file == "" && dir == "" {
		return fmt.Errorf("either --file or --dir is required")
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, cfg.AppsDir)

	if file != "" {
		if name == "" {
			return fmt.Errorf("--name is required with --file")
		}
		// copy compose file to apps dir
		absFile, _ := filepath.Abs(file)
		destDir := filepath.Join(cfg.AppsDir, name)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("create app dir: %w", err)
		}
		data, err := os.ReadFile(absFile)
		if err != nil {
			return fmt.Errorf("read compose file: %w", err)
		}
		dest := filepath.Join(destDir, "docker-compose.yml")
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("write compose file: %w", err)
		}
		if err := rec.DeployOne(cmd.Context(), dest, name); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
		fmt.Printf("deployed %s\n", name)
		return nil
	}

	// dir mode: deploy all subdirectories
	if err := os.MkdirAll(cfg.AppsDir, 0755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		appName := entry.Name()
		composePath := filepath.Join(dir, appName, "docker-compose.yml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			continue
		}
		// copy to apps dir
		data, _ := os.ReadFile(composePath)
		destDir := filepath.Join(cfg.AppsDir, appName)
		os.MkdirAll(destDir, 0755)
		os.WriteFile(filepath.Join(destDir, "docker-compose.yml"), data, 0644)

		if err := rec.DeployOne(cmd.Context(), filepath.Join(destDir, "docker-compose.yml"), appName); err != nil {
			fmt.Fprintf(os.Stderr, "error deploying %s: %v\n", appName, err)
			continue
		}
		fmt.Printf("deployed %s\n", appName)
	}
	return nil
}

func runRemove(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	name, _ := cmd.Flags().GetString("name")

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, cfg.AppsDir)

	if err := rec.RemoveOne(cmd.Context(), name); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}

	// remove from apps dir
	os.RemoveAll(filepath.Join(cfg.AppsDir, name))

	fmt.Printf("removed %s\n", name)
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	apps, err := db.ListApps()
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}

	if len(apps) == 0 {
		fmt.Println("no apps deployed")
		return nil
	}

	fmt.Printf("%-20s %-10s %-30s\n", "NAME", "STATUS", "DOMAIN")
	for _, app := range apps {
		fmt.Printf("%-20s %-10s %-30s\n", app.Name, app.Status, app.Domain)
	}
	return nil
}
```

Update `runServe` to wire the reconciler and start the watcher:

```go
func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(cfg.AppsDir, 0755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	if err := dc.Ping(cmd.Context()); err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, cfg.AppsDir)

	// start watcher in background
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	go func() {
		if err := rec.Watch(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}()

	srv := api.NewServer(cfg.ManagementPort)
	fmt.Printf("simpledeploy listening on :%d\n", cfg.ManagementPort)
	return srv.ListenAndServe()
}
```

Add `"context"` to imports and add imports for deployer and reconciler packages.

- [ ] **Step 2: Verify build**

Run: `make build`
Expected: exit 0

- [ ] **Step 3: Verify CLI help**

Run: `./bin/simpledeploy --help`
Expected: shows apply, remove, list, serve, init commands

- [ ] **Step 4: Commit**

```bash
git add cmd/simpledeploy/main.go
git commit -m "add apply, remove, list CLI commands and wire reconciler"
```

---

### Task 7: API Endpoints for Apps + Final Wiring

**Files:**
- Modify: `internal/api/server.go`
- Create: `internal/api/apps.go`
- Create: `internal/api/apps_test.go`

- [ ] **Step 1: Write failing tests for app endpoints**

Create `internal/api/apps_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv := NewServer(0, s)
	return srv, s
}

func TestListAppsEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	s.UpsertApp(&store.App{Name: "app1", Slug: "app1", ComposePath: "/tmp/1.yml", Status: "running", Domain: "app1.example.com"}, nil)
	s.UpsertApp(&store.App{Name: "app2", Slug: "app2", ComposePath: "/tmp/2.yml", Status: "stopped"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var apps []store.App
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 2 {
		t.Errorf("got %d apps, want 2", len(apps))
	}
}

func TestGetAppEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	s.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/1.yml", Status: "running", Domain: "myapp.example.com"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var app store.App
	json.NewDecoder(w.Body).Decode(&app)
	if app.Name != "myapp" {
		t.Errorf("Name = %q, want myapp", app.Name)
	}
}

func TestGetAppNotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestListAppsEmpty(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var apps []store.App
	json.NewDecoder(w.Body).Decode(&apps)
	if len(apps) != 0 {
		t.Errorf("got %d apps, want 0", len(apps))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -v`
Expected: FAIL - `NewServer` signature changed

- [ ] **Step 3: Update server to accept store dependency**

Update `internal/api/server.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vazra/simpledeploy/internal/store"
)

type Server struct {
	mux   *http.ServeMux
	port  int
	store *store.Store
}

func NewServer(port int, store *store.Store) *Server {
	s := &Server{
		mux:   http.NewServeMux(),
		port:  port,
		store: store,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/apps", s.handleListApps)
	s.mux.HandleFunc("GET /api/apps/{slug}", s.handleGetApp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, s.mux)
}
```

- [ ] **Step 4: Implement app handlers**

Create `internal/api/apps.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.ListApps()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if apps == nil {
		apps = []store.App{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app)
}
```

Note: `apps.go` needs to import `"github.com/vazra/simpledeploy/internal/store"` for the `store.App` type in `handleListApps`. Add the import.

- [ ] **Step 5: Update health test for new NewServer signature**

Update `TestHealthEndpoint` and `TestUnknownRouteReturns404` in `internal/api/server_test.go` to pass `nil` as store (health endpoint doesn't use it):

```go
func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(0, nil)
	// ... rest unchanged
}

func TestUnknownRouteReturns404(t *testing.T) {
	srv := NewServer(0, nil)
	// ... rest unchanged
}
```

- [ ] **Step 6: Update main.go to pass store to NewServer**

In `cmd/simpledeploy/main.go`, change:
```go
srv := api.NewServer(cfg.ManagementPort)
```
to:
```go
srv := api.NewServer(cfg.ManagementPort, db)
```

- [ ] **Step 7: Run all tests**

Run: `go test ./... -v -timeout 30s`
Expected: all tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/api/ cmd/simpledeploy/main.go
git commit -m "add app list/get API endpoints and wire store into server"
```

---

### Task 8: Final Tidy and Full Verification

**Files:**
- No new files

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -timeout 30s`
Expected: all tests pass across all packages

- [ ] **Step 2: Tidy dependencies**

Run: `go mod tidy`

- [ ] **Step 3: Verify clean build**

Run: `make clean && make build`
Expected: binary builds successfully

- [ ] **Step 4: Quick smoke test**

```bash
# generate config
./bin/simpledeploy init --config /tmp/sd-test.yaml

# list apps (should be empty)
./bin/simpledeploy list --config /tmp/sd-test.yaml

# verify --help shows new commands
./bin/simpledeploy --help
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "tidy dependencies for Phase 2"
```

---

## Verification Checklist

At the end of Phase 2, you should have:

- [ ] Compose files parsed with labels extracted (domain, port, TLS, backup, alerts, rate limit)
- [ ] Docker client supports full container lifecycle (create/start/stop/remove, networks, image pull)
- [ ] Store has app CRUD with labels (upsert, get, list, delete, update status)
- [ ] Deployer translates compose specs to Docker API calls
- [ ] Reconciler diffs desired vs actual state, deploys/removes as needed
- [ ] Directory watcher triggers reconciliation on changes with debounce
- [ ] `simpledeploy apply -f compose.yml --name myapp` deploys an app
- [ ] `simpledeploy apply -d ./` deploys all apps in directory
- [ ] `simpledeploy remove --name myapp` tears down an app
- [ ] `simpledeploy list` shows apps with status
- [ ] `GET /api/apps` returns list of apps
- [ ] `GET /api/apps/{slug}` returns app details
- [ ] All tests pass with `go test ./...`
