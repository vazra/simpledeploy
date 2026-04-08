# Phase 10: Client CLI - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remote client CLI for managing simpledeploy from a local machine. Context system (like kubectl) for multiple instances. Commands talk to remote API via HTTP. Apply/pull/diff/sync for config-as-code workflow.

**Architecture:** A client package wraps HTTP calls to the simpledeploy API. Context config stored at `~/.simpledeploy/config.yaml`. Existing CLI commands (apply, remove, list, etc.) gain a `--remote` mode that uses the HTTP client instead of direct store/docker access. New commands: context, diff, sync, pull.

**Tech Stack:** net/http client, existing API endpoints, YAML config for contexts

---

## File Structure

```
internal/client/client.go           - HTTP client for simpledeploy API
internal/client/client_test.go
internal/client/context.go          - Context config management
internal/client/context_test.go

internal/api/deploy.go              - Server-side deploy endpoint (upload compose)
internal/api/deploy_test.go

cmd/simpledeploy/main.go            - Context commands, remote-aware apply/list/etc
```

---

### Task 1: HTTP Client

**Files:**
- Create: `internal/client/client.go`
- Create: `internal/client/client_test.go`

#### Client:
```go
type Client struct {
    baseURL string
    apiKey  string
    http    *http.Client
}

func New(baseURL, apiKey string) *Client

// Apps
func (c *Client) ListApps() ([]store.App, error)
func (c *Client) GetApp(slug string) (*store.App, error)
func (c *Client) DeployApp(name string, composeData []byte) error  // POST compose file to server
func (c *Client) RemoveApp(slug string) error

// Logs
func (c *Client) StreamLogs(ctx context.Context, slug string, follow bool, tail string) (io.ReadCloser, error)

// Backups
func (c *Client) TriggerBackup(slug string) error
func (c *Client) ListBackupRuns(slug string) ([]store.BackupRun, error)
func (c *Client) Restore(runID int64) error

// Metrics
func (c *Client) GetAppMetrics(slug, from, to string) (json.RawMessage, error)
func (c *Client) GetSystemMetrics(from, to string) (json.RawMessage, error)
```

All methods set `Authorization: Bearer {apiKey}` header. Return parsed JSON responses or errors.

For DeployApp: `POST /api/apps/deploy` with body `{"name": "myapp", "compose": "base64-encoded-compose-file"}`.

#### Tests (use httptest.NewServer):
- TestClientListApps - mock server, verify request + parse response
- TestClientDeployApp - verify POST body
- TestClientAuthHeader - verify Bearer token sent

- [ ] Commit: `git commit -m "add HTTP client for remote API access"`

---

### Task 2: Server-Side Deploy Endpoint

**Files:**
- Create: `internal/api/deploy.go`
- Create: `internal/api/deploy_test.go`
- Modify: `internal/api/server.go`

The client needs a server endpoint to upload compose files for deployment.

#### Endpoint:
```
POST /api/apps/deploy
Body: {"name": "myapp", "compose": "<base64-encoded compose file>"}
```

Handler:
1. Decode base64 compose data
2. Write to apps_dir/{name}/docker-compose.yml
3. Trigger reconciler DeployOne
4. Return 201 with app status

The server needs access to the reconciler. Add setter like backup scheduler:
```go
func (s *Server) SetReconciler(rec *reconciler.Reconciler) { s.reconciler = rec }
```

Route:
```go
s.mux.Handle("POST /api/apps/deploy", s.authMiddleware(http.HandlerFunc(s.handleDeploy)))
```

Also add `DELETE /api/apps/{slug}` for remote remove:
```go
s.mux.Handle("DELETE /api/apps/{slug}", s.authMiddleware(
    s.appAccessMiddleware(http.HandlerFunc(s.handleRemoveApp))))
```

#### Tests:
- TestDeployEndpoint - POST compose, verify app created
- TestRemoveAppEndpoint - deploy then remove, verify gone

- [ ] Commit: `git commit -m "add deploy and remove API endpoints"`

---

### Task 3: Context Management

**Files:**
- Create: `internal/client/context.go`
- Create: `internal/client/context_test.go`

#### Context config (`~/.simpledeploy/config.yaml`):
```go
type ClientConfig struct {
    Contexts       map[string]Context `yaml:"contexts"`
    CurrentContext string             `yaml:"current_context"`
}

type Context struct {
    URL    string `yaml:"url"`
    APIKey string `yaml:"api_key"`
}

func LoadClientConfig() (*ClientConfig, error)
// Reads from ~/.simpledeploy/config.yaml

func SaveClientConfig(cfg *ClientConfig) error
// Writes to ~/.simpledeploy/config.yaml

func (cfg *ClientConfig) GetCurrentContext() (*Context, error)
func (cfg *ClientConfig) AddContext(name, url, apiKey string)
func (cfg *ClientConfig) UseContext(name string) error
```

#### Tests:
- TestAddAndGetContext
- TestUseContext
- TestGetCurrentContext
- TestLoadSaveConfig (temp file)

- [ ] Commit: `git commit -m "add client context management"`

---

### Task 4: CLI Commands (context, diff, sync, pull, remote-aware apply)

**Files:**
- Modify: `cmd/simpledeploy/main.go`

#### New commands:

**Context management:**
```
simpledeploy context add <name> --url <url> --api-key <key>
simpledeploy context use <name>
simpledeploy context list
```

**Remote-aware apply:**
Current apply works locally (direct docker + store). Add `--remote` flag or auto-detect: if `~/.simpledeploy/config.yaml` exists with a current context, use remote mode. If `--config` points to a server config, use local mode.

Simpler approach: keep existing commands as local-only (for the server). Add new remote commands:

```
simpledeploy remote apply -f compose.yml --name myapp
simpledeploy remote list
simpledeploy remote remove --name myapp
simpledeploy remote logs myapp --follow
```

Or even simpler: detect mode by checking if `--config` flag is set (local/server mode) vs if context exists (remote/client mode). But this is error-prone.

**Simplest approach:** The existing apply/remove/list commands work locally. For remote, the user runs:
```bash
simpledeploy --context production apply -f compose.yml --name myapp
```
Or sets up a context and commands auto-use it. If a context is active and the command isn't `serve`/`init`, use remote mode.

Actually, let's keep it pragmatic. Add these commands:
```
simpledeploy context add/use/list    # context management
simpledeploy pull --app myapp -o ./  # pull remote state to local files
simpledeploy diff --app myapp        # diff local vs remote
simpledeploy sync -d ./              # full sync local dir to remote
```

And make apply/remove/list use remote when `--context` is specified or a default context exists.

#### Implementation approach:

Add a helper that creates either a local client (direct store/docker) or remote client (HTTP):
```go
func getClient(cmd *cobra.Command) (*client.Client, error) {
    ctx, _ := cmd.Flags().GetString("context")
    if ctx != "" || hasDefaultContext() {
        cfg, _ := client.LoadClientConfig()
        context, _ := cfg.GetCurrentContext()
        return client.New(context.URL, context.APIKey), nil
    }
    return nil, nil // nil means local mode
}
```

Then in runApply/runList/runRemove: check if remote client exists, use it. Otherwise fall back to local.

#### Pull command:
```
simpledeploy pull --app myapp -o ./
simpledeploy pull --all -o ./
```
- GET /api/apps/{slug} to get app info
- Need a new endpoint to GET the compose file content: `GET /api/apps/{slug}/compose`
- Write to local file

#### Diff command:
```
simpledeploy diff --app myapp
```
- Read local compose file
- GET remote compose file
- Run diff

#### Sync command:
```
simpledeploy sync -d ./
```
- Scan local directory for apps
- GET remote app list
- Deploy new/changed apps
- Remove apps not in local dir

- [ ] Run full test suite, tidy, build
- [ ] Commit: `git commit -m "add context management, pull, diff, sync CLI commands"`

---

## Verification Checklist

- [ ] HTTP client wraps all API endpoints with auth
- [ ] Context config at ~/.simpledeploy/config.yaml
- [ ] `context add/use/list` commands
- [ ] Deploy endpoint accepts compose file upload
- [ ] `pull --app/--all` exports remote state to local files
- [ ] `diff --app` shows differences between local and remote
- [ ] `sync -d ./` deploys local changes and removes missing apps
- [ ] Apply/remove/list work in remote mode with --context
- [ ] All tests pass
