---
title: Small VPS tuning
description: Configuration tweaks for running SimpleDeploy on 1 vCPU / 1 GB hosts.
---

SimpleDeploy itself is light (about 60 MB resident for 10-20 small apps). On a 1 vCPU / 1 GB VPS, the limits you hit are usually Docker, the apps, and the SQLite write cadence rather than SimpleDeploy. A few config choices keep things smooth.

## Recommended starting specs

- **1 vCPU, 1 GB RAM, 25 GB disk**: minimum for a couple of nginx-class apps.
- **2 vCPU, 2 GB RAM, 40 GB disk**: comfortable for 10-20 small services.
- **4 vCPU, 4 GB RAM, 80 GB disk**: room for stateful services (Postgres, Redis) plus apps.

Add 10 GB headroom on disk for image churn.

## Trim metrics retention

`metrics.tiers` defaults are generous. On a tiny host, shorten the long tail:

```yaml
metrics:
  tiers:
    - { name: raw, interval: 10s,  retention: 30m }
    - { name: 1m,  interval: 1m,   retention: 12h }
    - { name: 5m,  interval: 5m,   retention: 3d }
    - { name: 1h,  interval: 1h,   retention: 30d }
    - { name: 1d,  interval: 1d,   retention: 180d }
```

Reduces DB size and write volume meaningfully.

## Cap the log buffer

```yaml
log_buffer_size: 200
```

Default is 500. On a small host with no remote log shipping, 200 is enough to debug recent issues.

## Tune rate limits

Defaults assume a beefier host. On 1 vCPU:

```yaml
ratelimit:
  requests: 100
  window: 60s
  burst: 25
```

This blocks bursty scrapers that would otherwise pin CPU.

## Prune Docker regularly

Old images and dangling layers eat disk fast. Use the dashboard's **Docker > Prune** or a weekly cron:

```bash
docker image prune -a -f --filter "until=168h"
```

## Vacuum the database

SQLite reclaims space on `VACUUM`. The system DB backup includes a `compact` mode that strips metrics and stats before vacuuming. Schedule it weekly.

## Avoid heavy backups on the same host

`pg_dump` of a multi-GB Postgres on a 1 GB host will swap. Push backups straight to S3 to avoid local intermediate files.

## Watch the right metrics

On small hosts: disk used, swap used, deploy duration. CPU and memory are visible in the dashboard but disk is the silent killer.
