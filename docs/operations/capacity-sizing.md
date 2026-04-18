---
title: Capacity and sizing
description: Database growth, per-app row counts, retention tradeoffs, and tuning guidance for SimpleDeploy's SQLite metrics store.
---

SimpleDeploy stores all metrics in a single SQLite database at `{data_dir}/simpledeploy.db`. Understanding the sizing factors helps with capacity planning.

**How rollup works:** Raw metrics are collected every 10s per container, then aggregated into coarser tiers (1m, 5m, 1h) and deleted from the source tier. This keeps the database compact while preserving long-term trends. Disk space freed by deleted rows is reclaimed automatically once per day via incremental vacuum.

**Per-app steady-state row counts** (assuming ~3 containers per app, default retention):

| Tier | Rows per app | Notes |
|------|-------------|-------|
| `raw` | ~26K | 3 containers x 6 points/min x 24h, constantly rotating |
| `1m` | ~30K | 3 containers x 1,440/day x 7 days |
| `5m` | ~26K | 3 containers x 288/day x 30 days |
| `1h` | ~26K | 3 containers x 24/day x 365 days |
| **Total** | **~108K** | |

Each row is roughly 150 bytes including indexes.

**Estimated database size by app count** (default retention, ~3 containers/app):

| Apps | Metric rows | DB size |
|------|------------|---------|
| 5 | ~540K | 80-100 MB |
| 10 | ~1.1M | 150-200 MB |
| 20 | ~2.2M | 300-400 MB |
| 50 | ~5.4M | 800 MB - 1 GB |

**Factors that increase size:**

- **More containers per app.** An app with 10 services generates 3x more rows than one with 3. This is the biggest multiplier.
- **Longer retention.** Doubling `1h` retention from 1 year to 2 years adds ~26K rows per app.
- **Request stats.** The `request_stats` table follows the same tiered rollup. High-traffic apps with many distinct endpoint patterns generate more rows.
- **Shorter raw interval.** Changing collection from 10s to 5s doubles raw tier throughput (though raw is pruned quickly).

**Factors that do NOT significantly affect size:**

- Number of proxied domains per app (metrics are per-container, not per-domain).
- Backup configurations (stored as config rows, not time-series).

**Reducing database size:**

- Lower retention on tiers you don't need. For most setups, `raw: 12h` and `1h: 90d` is sufficient.
- Run `VACUUM;` manually via `sqlite3 {data_dir}/simpledeploy.db "VACUUM;"` if the database grew large before upgrading to a version with automatic space reclamation.

Example config for a smaller footprint:

```yaml
metrics:
  tiers:
    - name: raw
      interval: 10s
      retention: 12h
    - name: 1m
      retention: 3d
    - name: 5m
      retention: 14d
    - name: 1h
      retention: 2160h  # 90 days
```

## Host sizing

Rough guidance for the host running SimpleDeploy itself (excluding what your apps need):

| Apps | CPU | RAM | Disk |
|------|-----|-----|------|
| 1-5 | 1 vCPU | 512 MB | 10 GB |
| 5-20 | 2 vCPU | 1 GB | 25 GB |
| 20-50 | 2-4 vCPU | 2 GB | 50 GB |
| 50+ | 4+ vCPU | 4 GB+ | 100 GB+ |

Add app-specific resource needs on top of these floors. The metrics collector and Caddy proxy together stay under 200 MB resident in normal operation; the rest is headroom for spikes during deploys (image pulls, container starts).

## Disk headroom

Plan for **3x your steady-state DB size** of free disk:

- 1x for the live DB
- 1x for the WAL during writes
- 1x for `VACUUM INTO` snapshots (DB backups copy the whole file)

If disk fills up, SQLite blocks writes and SimpleDeploy stops collecting metrics. Apps keep running.

See also: [Configuration reference](/reference/configuration/), [Small VPS tuning](/guides/small-vps-tuning/).
