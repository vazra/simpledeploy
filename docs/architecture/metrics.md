---
title: Metrics pipeline
description: Collection, tiered rollup, retention, and request stats.
---

The `internal/metrics/` package collects system and container metrics and stores them in tiered, downsampled form so the database stays bounded over years of operation.

## Collection

A collector goroutine runs every 10 seconds. Each tick it gathers:

- Host CPU, memory, disk via `gopsutil`.
- Per-container CPU, memory, network I/O, block I/O via `docker stats` (single API call, all containers).
- Process info for the simpledeploy binary itself.

Samples are pushed to a buffered Go channel.

## Writer

A single writer goroutine drains the channel in batches (every second or when the batch fills) and inserts into the `metrics` table in one transaction. Batching keeps SQLite write amplification low.

## Tiered rollup

A separate rollup job aggregates raw samples into coarser tiers:

| Tier | Interval | Retention (default) |
| --- | --- | --- |
| raw | 10s | 90 minutes |
| 1m | 1 minute | 24 hours |
| 5m | 5 minutes | 7 days |
| 1h | 1 hour | 90 days |
| 1d | 1 day | 400 days |

Each tier stores avg, min, and max for the underlying interval. Older rows are pruned after they roll up.

Tiers and retention are configurable in `config.yaml` under `metrics.tiers`.

## Query

The metrics API picks the lowest-resolution tier whose retention covers the requested time range. A 7-day chart reads from `5m`; a 1-year chart reads from `1d`. This keeps queries fast regardless of range.

## Request stats

A separate code path in the proxy package (the `simpledeploy_metrics` Caddy module) records every HTTP request: app slug, method, path, status, latency. These rows go into `request_stats` and follow a similar tier and prune cadence.

## Pruning

Pruning runs nightly. Manual prune is exposed at `POST /api/system/prune/{metrics,request-stats}` for emergencies.
