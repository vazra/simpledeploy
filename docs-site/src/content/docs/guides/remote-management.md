---
title: Remote management
description: Manage remote SimpleDeploy servers from your laptop with contexts, pull, diff, and sync, similar to kubectl.
---

The `simpledeploy` CLI doubles as a remote client. Configure server contexts, then use `pull`, `diff`, and `sync` to keep local compose files in sync with one or more servers.

## Contexts

Manage remote server connections.

```bash
# Add a context
simpledeploy context add production --url https://manage.example.com --api-key sd_...

# Switch context
simpledeploy context use staging

# List contexts (* = active)
simpledeploy context list
```

Contexts live in `~/.simpledeploy/config.yaml`.

## `simpledeploy pull`

Export remote app config to local files.

```bash
simpledeploy pull --app myapp -o ./
simpledeploy pull --all -o ./apps/
```

Downloads compose files from the remote server.

## `simpledeploy diff`

Compare local config vs remote state.

```bash
simpledeploy diff --app myapp
simpledeploy diff -d ./apps/
```

Shows line-by-line differences between local compose files and remote.

## `simpledeploy sync`

Sync a local directory to the remote server.

```bash
simpledeploy sync -d ./apps/
```

Deploys new/changed apps and removes apps not present locally.

## Workflow

A typical GitOps-ish workflow:

1. `simpledeploy pull --all -o ./apps/` to seed a local directory
2. Commit `./apps/` to a Git repo
3. Edit compose files locally, `simpledeploy diff -d ./apps/` to preview
4. `simpledeploy sync -d ./apps/` to apply

For CI-driven deploys, see [GitHub Actions integration](/integrations/github-actions/).
