---
title: CLI (handwritten backup)
description: Original handwritten CLI reference, preserved before auto-generation. The auto-generated cli.md supersedes this page.
---

All commands use the `--config` flag for server config (default: `/etc/simpledeploy/config.yaml`). Remote commands use the active context from `~/.simpledeploy/config.yaml`.

## Server Commands

### `simpledeploy serve`

Start the SimpleDeploy server.

```bash
simpledeploy serve --config /etc/simpledeploy/config.yaml
```

Starts: reverse proxy (Caddy), management API, reconciler watcher, metrics collector, backup scheduler, alert evaluator.

### `simpledeploy init`

Generate a default config file.

```bash
simpledeploy init --config /etc/simpledeploy/config.yaml
```

## App Management

### `simpledeploy apply`

Deploy apps from compose files.

```bash
# Single app
simpledeploy apply -f docker-compose.yml --name myapp

# All apps in directory
simpledeploy apply -d ./apps/
```

Copies the compose file to the server's apps directory and triggers deployment.

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--file` | `-f` | with --name | Compose file path |
| `--dir` | `-d` | alt to -f | Directory of app subdirectories |
| `--name` | | with -f | App name/slug |

### `simpledeploy remove`

Remove a deployed app.

```bash
simpledeploy remove --name myapp
```

Stops containers, removes network, deletes from store and apps directory.

### `simpledeploy list`

List deployed apps with status.

```bash
simpledeploy list
```

Output:
```
NAME                 STATUS     DOMAIN
myapp                running    myapp.example.com
postgres             running
```

### `simpledeploy logs`

Stream container logs.

```bash
simpledeploy logs myapp
simpledeploy logs myapp --follow=false --tail 50 --service db
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--follow` | `-f` | `true` | Follow log output |
| `--tail` | | `100` | Number of historical lines |
| `--service` | | `web` | Compose service name |

## Remote Client Commands

### `simpledeploy context`

Manage remote server connections.

```bash
# Add a context
simpledeploy context add production --url https://manage.example.com --api-key sd_...

# Switch context
simpledeploy context use staging

# List contexts (* = active)
simpledeploy context list
```

### `simpledeploy pull`

Export remote app config to local files.

```bash
simpledeploy pull --app myapp -o ./
simpledeploy pull --all -o ./apps/
```

Downloads compose files from the remote server.

### `simpledeploy diff`

Compare local config vs remote state.

```bash
simpledeploy diff --app myapp
simpledeploy diff -d ./apps/
```

Shows line-by-line differences between local compose files and remote.

### `simpledeploy sync`

Sync a local directory to the remote server.

```bash
simpledeploy sync -d ./apps/
```

Deploys new/changed apps and removes apps not present locally.

## Backup Commands

### `simpledeploy backup run`

Trigger an immediate backup.

```bash
simpledeploy backup run --app myapp
```

### `simpledeploy backup list`

List backup runs for an app.

```bash
simpledeploy backup list --app myapp
```

### `simpledeploy restore`

Restore from a backup.

```bash
simpledeploy restore --app myapp --id 42
```

## User Management

### `simpledeploy users`

Manage users (requires direct database access).

```bash
# Create user
simpledeploy users create --username admin --password secret --role super_admin

# List users
simpledeploy users list

# Delete user
simpledeploy users delete --id 2
```

Roles: `super_admin` (full access), `admin` (per-app access), `viewer` (read-only per-app).

### `simpledeploy apikey`

Manage API keys.

```bash
# Create key (prints plaintext once)
simpledeploy apikey create --name "claude-code" --user-id 1

# List keys
simpledeploy apikey list --user-id 1

# Revoke key
simpledeploy apikey revoke --id 3
```

API keys authenticate CLI and API requests via `Authorization: Bearer sd_...` header.

## Registry Management

### `simpledeploy registry`

Manage private container registry credentials.

```bash
# Add a registry
simpledeploy registry add --name ghcr-org --url ghcr.io --username myuser --password mytoken

# List registries
simpledeploy registry list

# Remove a registry
simpledeploy registry remove ghcr-org
```

| Subcommand | Description |
|------------|-------------|
| `add` | Add registry credentials (encrypted with master_secret) |
| `list` | List configured registries with decrypted usernames |
| `remove` | Remove a registry by name |

#### `registry add` flags

| Flag | Required | Description |
|------|----------|-------------|
| `--name` | Yes | Registry identifier (referenced in config/labels) |
| `--url` | Yes | Registry URL (e.g., `ghcr.io`, `123456.dkr.ecr.us-east-1.amazonaws.com`) |
| `--username` | Yes | Registry username |
| `--password` | Yes | Registry password or token |

Credentials are encrypted with AES-256-GCM using the `master_secret` from config.

## Config Sidecar Commands

### `simpledeploy config export`

Write all config sidecars from the current DB state to disk. Useful to verify sidecar content or backfill after a manual DB edit.

```bash
simpledeploy config export
```

See [Config sidecars](/operations/config-sidecars/) for the file layout.

### `simpledeploy config import`

Rebuild the DB from on-disk sidecars. Normally auto-recovery on startup handles this; use this command when you want to force an import into a non-empty DB.

```bash
simpledeploy config import --force --wipe
```

| Flag | Description |
|------|-------------|
| `--force` | Required to import into a non-empty DB |
| `--wipe` | Truncate config tables before importing (destructive) |

## Git Sync Commands

### `simpledeploy git status`

Show current git sync status: remote, branch, last sync time, and any recent conflicts.

```bash
simpledeploy git status
```

### `simpledeploy git sync-now`

Trigger an immediate pull-and-apply cycle without waiting for the next poll interval.

```bash
simpledeploy git sync-now
```

See [Git sync](/operations/git-sync/) for setup.

## Utility Commands

### `simpledeploy version`

Print version, commit hash, and build date.

```bash
simpledeploy version
# simpledeploy 1.2.0 (commit: abc1234, built: 2026-04-08T18:00:00Z)
```

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `/etc/simpledeploy/config.yaml` | Server config file path |
| `--help` | | Show help |
