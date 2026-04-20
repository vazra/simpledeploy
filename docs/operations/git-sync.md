---
title: Git sync
description: How to keep your SimpleDeploy app configs in a git repository, synced automatically on every change.
---

import { Aside } from '@astrojs/starlight/components';

Git sync is optional and disabled by default.

When enabled, SimpleDeploy treats your `apps_dir` as a git working tree and commits every config change to a remote repository. Each deploy, env-var edit, or sidecar update triggers a commit and push within seconds. You can also pull remote changes back in, making it possible to manage deployments through git rather than (or alongside) the UI.

Git sync complements [config sidecars](./config-sidecars), which write app config to local YAML files. Think of sidecars as the local source of truth and git sync as the transport layer that makes that truth portable. Sidecars are what get committed; git sync is what moves them to a remote.

Git sync is **not** a replacement for database backup. It does not capture metrics, deploy history, audit logs, or any file outside `apps_dir`. Secrets stay local: `config.yml` (which holds password hashes and encrypted registry credentials) and the SQLite database are never committed.

## What gets committed

- `{apps_dir}/{slug}/docker-compose.yml`
- `{apps_dir}/{slug}/.env`
- `{apps_dir}/{slug}/simpledeploy.yml` (per-app sidecar: alert rules, backup configs, access)
- `{apps_dir}/_global.yml` (redacted global config: users, registries, webhooks without secrets)
- `.gitignore` (auto-generated allowlist that restricts commits to the above files)

Never committed:

- `{data_dir}/config.yml` (password hashes, encrypted credentials)
- `{data_dir}/simpledeploy.db` (SQLite database)
- Metrics, logs, and anything outside `apps_dir`

## Config block

Add a `git_sync:` block to your server `config.yml`:

```yaml
git_sync:
  enabled: true
  remote: git@github.com:owner/infra.git
  branch: main
  author_name: SimpleDeploy
  author_email: bot@example.com
  ssh_key_path: /etc/simpledeploy/gitsync_id_ed25519
  poll_interval: 60s
  webhook_secret: "your-webhook-secret"
```

| Field | Required | Description |
|---|---|---|
| `enabled` | yes | Set to `true` to start the sync worker. |
| `remote` | yes | Git remote URL (SSH or HTTPS). |
| `branch` | yes | Branch to push to and pull from. |
| `author_name` | no | Commit author name. Defaults to `SimpleDeploy`. |
| `author_email` | no | Commit author email. |
| `ssh_key_path` | one of | Path to an SSH private key. Mutually exclusive with `https_token`. |
| `https_username` | one of | HTTPS username. Used together with `https_token`. |
| `https_token` | one of | HTTPS token or password. Mutually exclusive with `ssh_key_path`. |
| `poll_interval` | no | How often to pull from remote. Default `60s`. |
| `webhook_secret` | no | HMAC secret for verifying GitHub-compatible webhook pushes. |

## Authentication

**SSH:** Set `ssh_key_path` to an Ed25519 or RSA private key on the server. Add the corresponding public key as a deploy key on the remote (GitHub: Settings > Deploy keys, with write access).

**HTTPS:** Set `https_username` and `https_token`. For GitHub, create a fine-grained personal access token with `Contents: Read and write` permission on the target repository.

## First run and adopting existing state

If the remote repository is empty, SimpleDeploy initializes a repo in `apps_dir`, commits current state, and pushes. This is the recommended starting point.

If the remote already has commits, SimpleDeploy refuses to push and surfaces an error in `git status` and on the Git Sync page in admin nav. You then have two options:

- **Adopt local state:** from an admin shell, `git push --force` from `apps_dir` to overwrite the remote with current local state.
- **Adopt remote state:** manually clone the remote, move the files into `apps_dir`, and restart the server so sidecars are imported.

Start with an empty remote whenever possible to avoid this decision.

<Aside type="caution">
Force-pushing overwrites remote history. Only do it when you are certain local state is the source of truth.
</Aside>

## Webhook setup (optional but recommended)

A webhook lets the remote trigger an immediate pull instead of waiting for the next poll.

**GitHub:**
1. Repository Settings > Webhooks > Add webhook.
2. Payload URL: `https://<your-server>/api/git/webhook`
3. Content type: `application/json`
4. Secret: the value of `webhook_secret` in your config.
5. Event: "Just the push event."

SimpleDeploy verifies the `X-Hub-Signature-256` header using your secret. Gitea and GitLab use the same header format and are also supported.

## Poll and webhook coexistence

The poll worker runs on `poll_interval` (default 60s) regardless of webhook configuration. When a webhook arrives, an immediate sync runs; the poll continues as a safety net. There is no harm in running both.

## Conflict behavior

Local state wins on conflict. If a remote change conflicts with a local change, SimpleDeploy logs the conflict to `alert_history` and surfaces it on the Git Sync page. The remote change is not applied.

Conflicts usually mean two operators edited the same file at the same time. To apply the remote change, re-enter it through the UI after reviewing what was lost.

## CLI

```bash
simpledeploy git status      # print worker status and last sync time
simpledeploy git sync-now    # one-shot pull-and-apply against current config
```

`sync-now` is useful after a credentials change or for a manual bootstrap without restarting the server.

## Disabling git sync

Set `enabled: false` (or remove the block). The sync worker stops. The `.git` directory and all local history remain in `apps_dir` untouched. Re-enabling picks up where it left off.

## See also

- [Config sidecars and sidecar-based recovery](./config-sidecars) - sidecar schema and local DR recovery without git.
