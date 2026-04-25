---
title: Monitoring and observability
description: How to monitor SimpleDeploy itself: server logs, metrics, alerts, and integrations with external observability tools.
---

import { Aside } from '@astrojs/starlight/components';

Monitor SimpleDeploy in three layers. Each catches a different class of failure.

## Layer 1: SimpleDeploy itself

The server emits structured logs and exposes its own health and metrics endpoints.

### Process logs

SimpleDeploy writes everything to stdout/stderr in JSON. When installed as a systemd service it lands in journald.

```bash
journalctl -u simpledeploy -f                  # tail live
journalctl -u simpledeploy --since "1 hour ago"
journalctl -u simpledeploy -p err              # errors only
```

For long-term retention, set `SystemMaxUse=2G` in `/etc/systemd/journald.conf` or ship to Loki/CloudWatch via `journalbeat`/`vector`.

### Activity & audit log

Every config change, deploy outcome, auth event, and system action is recorded in the persistent activity log. View it at System → Audit Log (global) or on the per-app Activity tab.

```
GET /api/activity?limit=100
GET /api/apps/{slug}/activity?limit=50
```

Default retention is 365 days. See [Activity & Audit Log](/operations/security-audit/) for retention configuration and export options.

### Health endpoint

```
GET /api/health
```

Returns `200 OK` if the process is alive and the database is reachable. Use this as the target for external uptime checks.

## Layer 2: Caddy access (per-app request stats)

The embedded Caddy instance counts every HTTP request and stores it in SQLite via the `simpledeploy_metrics` handler module. Available via:

```
GET /api/apps/{slug}/request-stats?range=24h
```

Returned data: requests/sec, p50/p95/p99 latency, status code breakdown, top paths. The dashboard renders this on the per-app page.

## Layer 3: App-internal metrics

SimpleDeploy collects container CPU, memory, network, and disk I/O for every container every 10 seconds. Per-app aggregates are queryable via:

```
GET /api/apps/{slug}/metrics?range=24h
GET /api/metrics/system?range=24h
```

For app-specific business metrics (queue depth, request count, etc.) expose a `/metrics` endpoint inside your container and scrape with Prometheus or similar. SimpleDeploy does not scrape app-internal endpoints.

## External monitoring

<Aside type="caution">
Do not rely solely on SimpleDeploy's own alert rules to tell you SimpleDeploy is down. If the process is dead, no alert will fire. You need an *external* observer.
</Aside>

### Uptime check

Point UptimeRobot, Better Uptime, Pingdom, or your own cron at:

```
https://manage.example.com/api/health
```

Frequency: 1 minute. Notify a different channel than your normal alerts (e.g., SMS, not Slack).

### Metrics scrape

If you run a Prometheus/VictoriaMetrics/InfluxDB stack, scrape the system metrics endpoint on a cron and convert to your line protocol:

```bash
# Example: scrape every minute and forward to Influx
curl -s -H "Authorization: Bearer $SD_API_KEY" \
  https://manage.example.com/api/metrics/system?range=1m \
  | jq -r '...' \
  | curl -XPOST "$INFLUX_URL/write" --data-binary @-
```

This gets you long-term retention beyond the built-in tiered rollup.

## Alerts

SimpleDeploy's alert evaluator (`internal/alerts/`) runs rules against collected metrics and dispatches webhook notifications. Wire webhooks to PagerDuty, Slack, Discord, or any HTTP endpoint.

Recommended rule set:

| Rule | Threshold | Window |
|------|-----------|--------|
| High CPU per app | >80% | 5 min |
| High memory per app | >85% of limit | 5 min |
| Low host disk | <15% free | any |
| App down | no metrics | 5 min |
| Backup failed | last run failed | immediate |

See [Alert webhooks](/guides/alerts/webhooks/) and [Alert rules](/guides/alerts/rules/).

## Securing the monitoring surface

<Aside type="danger">
Never expose `/api/*` to the public internet without authentication. Anyone hitting `/api/activity` or `/api/apps/*` without a valid session or API key gets `401`, but that still leaks existence and timing. Put the management UI behind a VPN or restrict by source IP if possible.
</Aside>

The `/api/health` endpoint is safe to expose publicly. Everything else requires auth.
