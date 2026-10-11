---
title: Alerts engine
description: Rule evaluation loop, webhook dispatch, history snapshots, SSRF protections.
---

The alerts package (`internal/alerts/`) evaluates user-defined rules against current metrics and dispatches webhooks when conditions hold for the configured duration.

## Rule shape

A rule is `(metric, op, threshold, duration_seconds, scope)`:

- **metric**: `cpu_pct` (CPU percent, 100 = one core), `mem_pct` (memory as a percent of the container limit), `mem_bytes` (memory in bytes). Backup event rules use `backup_failed`, `backup_success`, `backup_missed` or `backup_all` (any backup event); they fire when the backup scheduler reports that event for the rule's app, or for any app if the rule is global (see [Backup alerts](#backup-alerts)). The API rejects other metrics and operators.
- **op**: `>`, `<`, `>=`, `<=`
- **threshold**: numeric value (percent, count, etc.)
- **duration**: condition must hold continuously for this many seconds
- **scope**: per-app (`app_slug`) or global (`null`)

Rules live in the `alert_rules` table. Webhooks live in `webhooks`. Many rules can target the same webhook.

## Evaluator loop

The evaluator runs on a fixed tick (default every 30 seconds). On each tick it:

1. Loads active rules and snapshots current metrics from the metrics writer.
2. Computes whether the condition holds. If yes, increments an in-memory dwell counter; if no, resets it.
3. When dwell crosses the rule's `duration_seconds`, the alert fires once. Subsequent ticks while still firing do not re-fire.
4. When the condition clears, the rule re-arms.

Firings write to `alert_history` with a snapshot of the rule fields at firing time, so changes to the rule afterward do not rewrite history.

## Webhook dispatch

The dispatcher posts JSON to the webhook URL. Payload includes rule id, metric, threshold, observed value, app slug (if any), timestamp, and a human-readable message. See [the webhooks guide](/guides/alerts/webhooks/) for the full schema.

Templates (builtin and custom) are Go `text/template`s that render JSON. Before execution, `jsonSafeEvent` JSON-escapes every string field of the event (app name and slug, metric, display values, operator, status), so templates place fields directly inside quotes and app names with quotes, backslashes or newlines keep the payload valid. Numeric fields and `FiredAt` are passed as-is.

Network safety: by default the dispatcher refuses URLs that resolve to reserved addresses, and the dialer re-checks the resolved IP at connect time (DNS rebinding). `isReservedIP` covers loopback, RFC 1918, link-local, multicast, unspecified, CGNAT (`100.64.0.0/10`), IETF/TEST-NET/benchmarking ranges, class E, IPv6 unique-local and discard, and the NAT64 local-use prefix `64:ff9b:1::/48`. Addresses in the well-known NAT64 prefix `64:ff9b::/96` are judged by the IPv4 address they embed, so DNS64 hosts can still reach public targets. Set `SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS=1` to opt out (only for trusted internal targets). The S3 backup target uses a similar list (`SIMPLEDEPLOY_ALLOW_PRIVATE_S3`, see [Backup architecture](/architecture/backup/)).

Delivery is single-shot: each notification is one POST with a 10 second timeout. A failed send (network error or HTTP status >= 400) is logged and not retried. A metric alert still records its `alert_history` row, so it will not fire again until it resolves.

## Backup alerts

The backup scheduler reports events through a callback wired in `cmd/simpledeploy/main.go` to `Evaluator.DispatchBackupAlert`, so a single webhook can receive both metric and backup alerts. Event types are `backup_failed`, `backup_success` and `backup_missed` (from the periodic missed-schedule check). A rule matches when its metric equals the event type or is `backup_all`.

Scope: a global rule (no app) fires for every app's events. An app-scoped rule fires only for its own app: the scheduler passes the app's ID with each event and the dispatcher compares it with the rule's app.

Backup events have no threshold or duration and no `alert_history` row: every matching event sends one notification. The payload uses `Status` `firing` (the rule fired) for every event type, `MetricDisplay` `Backup Failed`, `Backup Succeeded` or `Backup Missed`, and the scheduler's message as `ValueDisplay`.
