---
title: Backup hooks
description: Run a command, or stop and start a service, before or after a backup or restore. Hooks only reach the services of the app being backed up.
---

Most backups need no hooks: the database strategies already take consistent dumps. Hooks are for extra steps, such as pausing a worker that writes to the files a volume backup copies.

Add them in step 4 of the Backup wizard, or set `pre_hooks` and `post_hooks` on the backup config through the API.

## Hook types

| Type | What it does |
|------|--------------|
| `stop` | Stops the service's containers. |
| `start` | Starts the service's containers. |
| `exec` | Runs `command` with `sh -c` inside the service's running container. |
| `flush_redis` | Runs `redis-cli BGSAVE` in the service. |
| `flush_mysql` | Runs `FLUSH TABLES WITH READ LOCK` then `UNLOCK TABLES` as MySQL `root` (no password) in the service. |

Each hook is `{"type", "service", "command", "timeout"}`. `pre_hooks` and `post_hooks` hold JSON lists:

```json
{
  "pre_hooks": "[{\"type\":\"stop\",\"service\":\"worker\"}]",
  "post_hooks": "[{\"type\":\"start\",\"service\":\"worker\"}]"
}
```

## Rules

- `service` is a service name from the app's compose file (for example `web`), not a container name. Hooks only reach this app's own containers (compose project `simpledeploy-<slug>`), never another app, SimpleDeploy itself, or other containers on the server.
- Up to 20 hooks per list. `timeout` is 0 to 3600 seconds (0 means the 30 second default). `command` is required for `exec` and limited to 4096 characters.
- A pre-backup hook cannot stop the service the backup runs in: the backup reads (and a restore writes) through that container, so it must keep running. Stop the services that write to its data instead.
- Invalid hooks are refused when the config is saved, with the reason. Hooks that arrive through config files or git sync are checked again before each run; an invalid one fails the run before anything executes.

## When hooks run

- Backup: pre-hooks, backup, upload, post-hooks.
- Restore: download and checksum check, pre-hooks, restore, post-hooks.
- Pre-hooks run in order. The first failure cancels the backup and marks the run failed. Post-hooks still run, so a service a pre-hook stopped is started again.
- A failed post-hook is logged as a warning and does not fail the backup.

## Security

`exec` commands run inside the app's own container with `sh -c`, with the access that container already has. Users who can change an app's backup settings can already change what runs in that app (its `.env`, compose versions, and restores into its containers), and compose validation keeps app containers away from the host. Hooks never run commands on the host or in another app's containers.

## Hooks from older versions

The "Stop container during backup" and "Flush to disk before backup" options of older Backup wizard versions saved hooks in a format that never ran. They are skipped (with a log line) and removed the next time the config is saved.

See also: [Backups overview](/guides/backups/overview/), [Restore](/guides/backups/restore/).
