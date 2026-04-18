---
title: Alert webhooks
description: Configure outbound webhooks for alert delivery. Built-in templates for Slack, Telegram, Discord, plus a generic custom mode.
---

Webhooks are the delivery channel for alert rules. Create one, then reference it from one or more rules.

## Add a webhook

```bash
curl -X POST https://manage.yourdomain.com/api/webhooks \
  -H "Authorization: Bearer sd_..." \
  -H "Content-Type: application/json" \
  -d '{"name":"slack","type":"slack","url":"https://hooks.slack.com/services/..."}'
```

| Field | Notes |
|-------|-------|
| `name` | Friendly identifier |
| `type` | `slack`, `telegram`, `discord`, or `custom` |
| `url` | Target URL (https only by policy) |
| `template_override` | Optional Go template for custom payload (super_admin only) |
| `headers_json` | Optional JSON object of additional headers |

## Security

Outbound webhooks are SSRF-protected: only `http://` and `https://` schemes, blocked private/loopback/metadata IPs, and allow-listed headers. See [Security hardening](/operations/security-hardening/) for details.

See also: [Alert rules](/guides/alerts/rules/), [Recipes](/guides/alerts/recipes/).
