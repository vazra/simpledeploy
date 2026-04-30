---
title: Directory layout
description: Where SimpleDeploy keeps its config, database, app definitions, backups, and Caddy state on disk.
---

SimpleDeploy uses three directories on the server: a config directory, a data directory (mutable runtime state), and an apps directory (compose files watched by the reconciler). Defaults below; all paths are configurable in `config.yaml`.

## Config directory

```
/etc/simpledeploy/
├── config.yaml         # server config (YAML)
└── apps/               # apps_dir, see below
```

The config file is read once at startup. `master_secret` inside it must be present and stable across restarts (it derives encryption keys for stored credentials and JWT signing).

Permissions: `0640 root:simpledeploy` for `config.yaml`. The file contains the master secret in cleartext.

## Data directory (`data_dir`)

Default: `/var/lib/simpledeploy/`.

```
/var/lib/simpledeploy/
├── simpledeploy.db        # SQLite database (apps, users, metrics, alerts, backups)
├── simpledeploy.db-wal    # SQLite write-ahead log (transient)
├── simpledeploy.db-shm    # SQLite shared-memory index (transient)
├── backups/               # local backup target output
│   └── <app-slug>/
│       └── 2026-04-17T02-00-00Z.sql.gz
└── caddy/                 # Caddy storage (ACME state, certs, locks)
    ├── certificates/
    └── locks/
```

Caddy storage lives under `data_dir/caddy/` for both `tls.mode: auto` (Let's Encrypt ACME state and issued certs) and `tls.mode: local` (self-signed CA). For `tls.mode: custom`, you provide cert files and SimpleDeploy reads them from the path declared by each app.

Permissions:

| Path | Owner | Mode |
|------|-------|------|
| `data_dir/` | `simpledeploy` | `0700` |
| `simpledeploy.db` | `simpledeploy` | `0600` |
| `backups/` | `simpledeploy` | `0700` |

## Apps directory (`apps_dir`)

Default: `/etc/simpledeploy/apps/`. Watched by the reconciler; one subdirectory per app.

```
/etc/simpledeploy/apps/
├── myapp/
│   ├── docker-compose.yml
│   └── .env                # optional, picked up by docker compose
├── api-service/
│   └── docker-compose.yml
└── postgres/
    └── docker-compose.yml
```

The directory name becomes the app slug (used in URLs, CLI commands, and metrics). Adding, modifying, or deleting a subdirectory triggers a reconcile within seconds.

Permissions: `0750 simpledeploy:simpledeploy` for the directory; compose files can be `0640`. `.env` files often contain secrets, so keep them at `0600`.

## Client config

Per-user, on the workstation running the CLI.

```
~/.simpledeploy/
└── config.yaml             # contexts (URL + API key per remote server)
```

Managed via `simpledeploy context add|use|list`. Permissions `0600` are recommended since API keys grant full server access.

## See also

- [Configuration](/reference/configuration/) for `data_dir` and `apps_dir` config keys.
- [Backups](/guides/backups/overview/) for the `backups/` layout in detail.
- [TLS and HTTPS](/guides/tls/) for Caddy storage paths.
