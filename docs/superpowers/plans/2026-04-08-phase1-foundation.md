# Phase 1: Foundation - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go binary that starts, reads config from YAML, connects to SQLite (WAL mode, migrations), connects to Docker, and serves a health endpoint on the management port.

**Architecture:** Single binary CLI using cobra. `serve` command wires config, store, Docker client, and HTTP API server. `init` command generates a default config file. All packages under `internal/` with clean interfaces.

**Tech Stack:** Go 1.22+, cobra (CLI), yaml.v3 (config), modernc.org/sqlite (pure Go, no CGO), docker/docker (Docker client), stdlib net/http (API)

---

## File Structure

```
cmd/simpledeploy/main.go                  - CLI entrypoint, cobra commands (serve, init)
internal/config/config.go                  - Config struct, Load(), DefaultConfig()
internal/config/config_test.go             - Config parsing tests
internal/store/store.go                    - SQLite connection, WAL setup, migration runner
internal/store/store_test.go               - Store open/migrate/close tests
internal/store/migrations/001_initial.sql  - Initial schema (apps table)
internal/docker/client.go                  - Docker client wrapper, Ping
internal/docker/client_test.go             - Interface conformance test
internal/api/server.go                     - HTTP server, health endpoint
internal/api/server_test.go                - Health endpoint test
Makefile                                   - Build, test, clean targets
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`, `Makefile`, `cmd/simpledeploy/main.go`

- [ ] **Step 1: Initialize Go module and install dependencies**

```bash
cd /Users/ameen/dev/vazra/simpledeploy
go mod init github.com/vazra/simpledeploy
go get github.com/spf13/cobra@latest
go get gopkg.in/yaml.v3@latest
go get modernc.org/sqlite@latest
go get github.com/docker/docker@latest
go get github.com/docker/go-connections@latest
```

- [ ] **Step 2: Create directory structure**

```bash
mkdir -p cmd/simpledeploy
mkdir -p internal/config
mkdir -p internal/store/migrations
mkdir -p internal/docker
mkdir -p internal/api
```

- [ ] **Step 3: Create minimal main.go**

Create `cmd/simpledeploy/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "simpledeploy",
	Short: "Lightweight deployment manager for Docker Compose apps",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Create Makefile**

Create `Makefile`:

```makefile
.PHONY: build test clean

build:
	go build -ldflags="-s -w" -o bin/simpledeploy ./cmd/simpledeploy

test:
	go test ./...

clean:
	rm -rf bin/
```

- [ ] **Step 5: Verify it compiles**

Run: `make build`
Expected: binary at `bin/simpledeploy`, exit 0

Run: `./bin/simpledeploy --help`
Expected: shows usage with `--config` flag

- [ ] **Step 6: Commit**

```bash
git add cmd/ internal/ go.mod go.sum Makefile
git commit -m "scaffold project structure with cobra CLI"
```

---

### Task 2: Config Parsing

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for config loading**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DataDir != "/var/lib/simpledeploy" {
		t.Errorf("DataDir = %q, want /var/lib/simpledeploy", cfg.DataDir)
	}
	if cfg.AppsDir != "/etc/simpledeploy/apps" {
		t.Errorf("AppsDir = %q, want /etc/simpledeploy/apps", cfg.AppsDir)
	}
	if cfg.ListenAddr != ":443" {
		t.Errorf("ListenAddr = %q, want :443", cfg.ListenAddr)
	}
	if cfg.ManagementPort != 8443 {
		t.Errorf("ManagementPort = %d, want 8443", cfg.ManagementPort)
	}
	if cfg.TLS.Mode != "auto" {
		t.Errorf("TLS.Mode = %q, want auto", cfg.TLS.Mode)
	}
	if len(cfg.Metrics.Tiers) != 4 {
		t.Errorf("Metrics.Tiers len = %d, want 4", len(cfg.Metrics.Tiers))
	}
	if cfg.RateLimit.Requests != 200 {
		t.Errorf("RateLimit.Requests = %d, want 200", cfg.RateLimit.Requests)
	}
}

func TestLoadConfig(t *testing.T) {
	yaml := `
data_dir: /tmp/sd-test
apps_dir: /tmp/sd-apps
listen_addr: ":8080"
management_port: 9090
domain: test.example.com
tls:
  mode: "off"
  email: test@example.com
master_secret: "test-secret"
metrics:
  tiers:
    - name: raw
      interval: 10s
      retention: 12h
ratelimit:
  requests: 100
  window: 30s
  burst: 20
  by: ip
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DataDir != "/tmp/sd-test" {
		t.Errorf("DataDir = %q, want /tmp/sd-test", cfg.DataDir)
	}
	if cfg.ManagementPort != 9090 {
		t.Errorf("ManagementPort = %d, want 9090", cfg.ManagementPort)
	}
	if cfg.TLS.Mode != "off" {
		t.Errorf("TLS.Mode = %q, want off", cfg.TLS.Mode)
	}
	if cfg.MasterSecret != "test-secret" {
		t.Errorf("MasterSecret = %q, want test-secret", cfg.MasterSecret)
	}
	if len(cfg.Metrics.Tiers) != 1 {
		t.Errorf("Metrics.Tiers len = %d, want 1", len(cfg.Metrics.Tiers))
	}
	if cfg.RateLimit.Burst != 20 {
		t.Errorf("RateLimit.Burst = %d, want 20", cfg.RateLimit.Burst)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL - `config` package doesn't exist yet

- [ ] **Step 3: Implement config**

Create `internal/config/config.go`:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DataDir        string          `yaml:"data_dir"`
	AppsDir        string          `yaml:"apps_dir"`
	ListenAddr     string          `yaml:"listen_addr"`
	ManagementPort int             `yaml:"management_port"`
	Domain         string          `yaml:"domain"`
	TLS            TLSConfig       `yaml:"tls"`
	MasterSecret   string          `yaml:"master_secret"`
	Metrics        MetricsConfig   `yaml:"metrics"`
	RateLimit      RateLimitConfig `yaml:"ratelimit"`
}

type TLSConfig struct {
	Mode  string `yaml:"mode"`
	Email string `yaml:"email"`
}

type MetricsTier struct {
	Name      string `yaml:"name"`
	Interval  string `yaml:"interval,omitempty"`
	Retention string `yaml:"retention"`
}

type MetricsConfig struct {
	Tiers []MetricsTier `yaml:"tiers"`
}

type RateLimitConfig struct {
	Requests int    `yaml:"requests"`
	Window   string `yaml:"window"`
	Burst    int    `yaml:"burst"`
	By       string `yaml:"by"`
}

func DefaultConfig() *Config {
	return &Config{
		DataDir:        "/var/lib/simpledeploy",
		AppsDir:        "/etc/simpledeploy/apps",
		ListenAddr:     ":443",
		ManagementPort: 8443,
		TLS: TLSConfig{
			Mode: "auto",
		},
		Metrics: MetricsConfig{
			Tiers: []MetricsTier{
				{Name: "raw", Interval: "10s", Retention: "24h"},
				{Name: "1m", Retention: "7d"},
				{Name: "5m", Retention: "30d"},
				{Name: "1h", Retention: "8760h"},
			},
		},
		RateLimit: RateLimitConfig{
			Requests: 200,
			Window:   "60s",
			Burst:    50,
			By:       "ip",
		},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "add config parsing with defaults and YAML loading"
```

---

### Task 3: SQLite Store

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`
- Create: `internal/store/migrations/001_initial.sql`

- [ ] **Step 1: Write failing test for store**

Create `internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	// verify WAL mode
	var journalMode string
	err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	// verify apps table exists by inserting a row
	_, err = s.db.Exec(
		`INSERT INTO apps (name, slug, compose_path, status) VALUES (?, ?, ?, ?)`,
		"test-app", "test-app", "/tmp/test/docker-compose.yml", "stopped",
	)
	if err != nil {
		t.Fatalf("insert into apps: %v", err)
	}

	// verify we can read it back
	var name, slug, status string
	err = s.db.QueryRow("SELECT name, slug, status FROM apps WHERE slug = ?", "test-app").
		Scan(&name, &slug, &status)
	if err != nil {
		t.Fatalf("select from apps: %v", err)
	}
	if name != "test-app" || status != "stopped" {
		t.Errorf("got name=%q status=%q, want test-app stopped", name, status)
	}
}

func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open() error: %v", err)
	}
	s1.Close()

	// opening again should not fail (migrations already applied)
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open() error: %v", err)
	}
	s2.Close()
}

func TestMigrationVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer s.Close()

	var version int
	err = s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if version != 1 {
		t.Errorf("migration version = %d, want 1", version)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL - package doesn't compile

- [ ] **Step 3: Create initial migration SQL**

Create `internal/store/migrations/001_initial.sql`:

```sql
CREATE TABLE IF NOT EXISTS apps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    compose_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped' CHECK(status IN ('running', 'stopped', 'error')),
    domain TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_apps_slug ON apps(slug);
CREATE INDEX IF NOT EXISTS idx_apps_status ON apps(status);
```

- [ ] **Step 4: Implement store**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA cache_size=2000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set cache size: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var filenames []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			filenames = append(filenames, e.Name())
		}
	}
	sort.Strings(filenames)

	for _, name := range filenames {
		version, err := parseVersion(name)
		if err != nil {
			return fmt.Errorf("parse version from %s: %w", name, err)
		}

		var exists int
		err = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if exists > 0 {
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(string(data)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func parseVersion(filename string) (int, error) {
	// expects format: 001_description.sql
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}
	var v int
	if _, err := fmt.Sscanf(parts[0], "%d", &v); err != nil {
		return 0, fmt.Errorf("invalid version in %s: %w", filename, err)
	}
	return v, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store/ -v`
Expected: all 3 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "add SQLite store with WAL mode and migration runner"
```

---

### Task 4: Docker Client Wrapper

**Files:**
- Create: `internal/docker/client.go`
- Create: `internal/docker/client_test.go`

- [ ] **Step 1: Write test for client interface**

Create `internal/docker/client_test.go`:

```go
package docker

import (
	"context"
	"testing"
)

// compile-time interface conformance check
var _ Client = (*DockerClient)(nil)

func TestNewClientDoesNotPanic(t *testing.T) {
	// NewClient may fail if Docker is not running, but it should not panic.
	// We test the constructor and Close() path.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/docker/ -v`
Expected: FAIL - package doesn't compile

- [ ] **Step 3: Implement Docker client**

Create `internal/docker/client.go`:

```go
package docker

import (
	"context"

	dockerclient "github.com/docker/docker/client"
)

type Client interface {
	Ping(ctx context.Context) error
	Close() error
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

func (c *DockerClient) Raw() *dockerclient.Client {
	return c.cli
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/docker/ -v`
Expected: PASS (tests skip gracefully if Docker is not running)

- [ ] **Step 5: Commit**

```bash
git add internal/docker/
git commit -m "add Docker client wrapper with Ping and interface"
```

---

### Task 5: API Server + Health Endpoint

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/server_test.go`

- [ ] **Step 1: Write failing test for health endpoint**

Create `internal/api/server_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(0)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	srv := NewServer(0)

	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -v`
Expected: FAIL - package doesn't compile

- [ ] **Step 3: Implement API server**

Create `internal/api/server.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Server struct {
	mux  *http.ServeMux
	port int
}

func NewServer(port int) *Server {
	s := &Server{
		mux:  http.NewServeMux(),
		port: port,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -v`
Expected: both tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "add API server with health endpoint"
```

---

### Task 6: CLI Commands (serve + init)

**Files:**
- Modify: `cmd/simpledeploy/main.go`

- [ ] **Step 1: Implement serve and init commands**

Replace `cmd/simpledeploy/main.go` with:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vazra/simpledeploy/internal/api"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/store"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "simpledeploy",
	Short: "Lightweight deployment manager for Docker Compose apps",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the simpledeploy server",
	RunE:  runServe,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default config file",
	RunE:  runInit,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")
	rootCmd.AddCommand(serveCmd, initCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
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

	srv := api.NewServer(cfg.ManagementPort)
	fmt.Printf("simpledeploy listening on :%d\n", cfg.ManagementPort)
	return srv.ListenAndServe()
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg := config.DefaultConfig()
	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(cfgFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("config written to %s\n", cfgFile)
	return nil
}
```

- [ ] **Step 2: Verify build**

Run: `make build`
Expected: exit 0

- [ ] **Step 3: Test init command**

Run: `./bin/simpledeploy init --config /tmp/sd-test-config.yaml`
Expected: "config written to /tmp/sd-test-config.yaml"

Run: `cat /tmp/sd-test-config.yaml`
Expected: YAML with default config values

- [ ] **Step 4: Test serve command (quick smoke test)**

Run: `./bin/simpledeploy serve --config /tmp/sd-test-config.yaml &`

Wait 1 second, then:
Run: `curl -s http://localhost:8443/api/health`
Expected: `{"status":"ok"}`

Kill the background process.

Note: this may fail if Docker is not running or port 8443 is in use. That's expected.

- [ ] **Step 5: Commit**

```bash
git add cmd/simpledeploy/main.go
git commit -m "wire serve and init CLI commands"
```

---

### Task 7: Full Test Suite + Tidy

**Files:**
- No new files

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: all tests pass across all packages

- [ ] **Step 2: Tidy dependencies**

Run: `go mod tidy`

- [ ] **Step 3: Verify clean build**

Run: `make clean && make build`
Expected: fresh binary, exit 0

Run: `ls -lh bin/simpledeploy`
Expected: binary exists (note the size for baseline)

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "tidy dependencies"
```

---

## Verification Checklist

At the end of Phase 1, you should have:

- [ ] `simpledeploy init --config <path>` generates a default YAML config
- [ ] `simpledeploy serve --config <path>` starts and serves health endpoint
- [ ] SQLite database created in data_dir with WAL mode
- [ ] Migrations applied (apps table exists)
- [ ] Docker connectivity verified on startup
- [ ] `GET /api/health` returns `{"status":"ok"}`
- [ ] All tests pass with `make test`
- [ ] Binary builds with `make build`
