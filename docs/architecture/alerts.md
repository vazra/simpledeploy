---
title: Alerts engine
description: Rule evaluation loop, webhook dispatch, history snapshots, SSRF protections.
---

The alerts package (`internal/alerts/`) evaluates user-defined rules against current metrics and dispatches webhooks when conditions hold for the configured duration.

## Rule shape

A rule is `(metric, op, threshold, duration_seconds, scope)`:

- **metric**: `cpu`, `memory`, `disk`, `request_rate`, `request_error_rate`, `deploy_failure`, `backup_failure`
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

Failed dispatches retry with exponential backoff up to a small bound; persistent failures are recorded but not retried indefinitely.

## Backup failure alerts

Backup runs that fail surface through the same alert pipeline using the `backup_failure` metric, so a single webhook can receive both metric and operational alerts.
