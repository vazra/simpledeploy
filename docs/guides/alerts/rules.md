---
title: Alert rules
description: Define threshold-based alert rules on app metrics like CPU and memory, with duration windows and webhook delivery.
---

Alert rules fire a webhook when a metric crosses a threshold for a given duration.

## Add an alert rule

```bash
curl -X POST https://manage.yourdomain.com/api/alerts/rules \
  -H "Authorization: Bearer sd_..." \
  -H "Content-Type: application/json" \
  -d '{"app_id":1,"metric":"cpu_pct","operator":">","threshold":80,"duration_sec":300,"webhook_id":1,"enabled":true}'
```

| Field | Type | Notes |
|-------|------|-------|
| `app_id` | int | Target app (omit / null for system-wide) |
| `metric` | string | `cpu_pct`, `mem_bytes`, `mem_pct` |
| `operator` | string | `>`, `<`, `>=`, `<=` |
| `threshold` | float | Numeric threshold |
| `duration_sec` | int | Sustained breach window before firing |
| `webhook_id` | int | Webhook to dispatch to |
| `enabled` | bool | Toggle without deleting |

## Compose label shortcuts

Default rules can be created automatically from compose labels:

```yaml
labels:
  simpledeploy.alerts.cpu: ">80,5m"
  simpledeploy.alerts.memory: ">90,5m"
```

Format: `{operator}{threshold},{duration}`. Rules created this way can be tuned or disabled later via the API/UI.

See also: [Webhooks](/guides/alerts/webhooks/), [Recipes](/guides/alerts/recipes/), [Compose labels](/reference/compose-labels/).
