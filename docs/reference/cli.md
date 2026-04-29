---
title: CLI
description: Command-line reference for the simpledeploy binary, covering server, app management, remote, backup, user, and registry commands.
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

See [Remote management](/guides/remote-management/) for `context`, `pull`, `diff`, and `sync` usage.

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

Roles: `super_admin` (full platform + all apps), `manage` (write access to granted apps only, no platform mgmt, cannot create or delete apps), `viewer` (read-only on granted apps).

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
