---
title: Audit log
description: Track logins, deploys, user changes, role grants, and API key operations. Available in the UI, via API, and on stderr as JSON.
---

import { Tabs, TabItem, Aside } from '@astrojs/starlight/components';

Every security-relevant action is logged to a rolling buffer plus stderr as JSON. Use it for incident review, compliance evidence, or just to check who deployed what.

## What gets logged

| Event | Triggered by |
|-------|--------------|
| `login` / `login_failed` | Auth attempts |
| `user_created` / `user_deleted` | User mgmt |
| `role_changed` | Role updates |
| `access_granted` / `access_revoked` | Per-app grants |
| `apikey_created` / `apikey_deleted` | API key lifecycle |
| `apikey_used` | First use, then sampled |
| `deploy` / `app_removed` | App lifecycle |
| `config_changed` | Env, endpoints, access, scaling |

Each entry includes timestamp, event type, username, source IP, success flag, and a free-form `detail` field.

## View the log

<Tabs>
<TabItem label="UI">
System, **Audit** tab. Filterable by event type, user, time range. Click a row for full JSON.
</TabItem>
<TabItem label="API">
```bash
curl https://manage.example.com/api/system/audit-log?limit=100 \
  -H "Authorization: Bearer $SD_API_KEY"
```
Returns newest-last array of events:
```json
[
  {"timestamp":"2026-04-17T10:30:00Z","type":"login","username":"alice","ip":"203.0.113.10","success":true},
  {"timestamp":"2026-04-17T10:31:00Z","type":"deploy","username":"alice","detail":"myapp","success":true}
]
```
</TabItem>
<TabItem label="stderr / journald">
The same JSON is written to stderr. With systemd:
```bash
journalctl -u simpledeploy -o cat | jq 'select(.type=="deploy")'
```
Ship it to Loki, CloudWatch, or any aggregator without extra config.
</TabItem>
</Tabs>

## Retention

Defaults to a 500-entry ring buffer in memory. Tune via API (super_admin only):

```bash
curl -X PUT https://manage.example.com/api/system/audit-config \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"retention_count": 5000}'
```

For long-term retention, ship the stderr stream to your log aggregator. The buffer is for quick lookup, not archival.

## Export

```bash
curl https://manage.example.com/api/system/audit-log?limit=5000 \
  -H "Authorization: Bearer $SD_API_KEY" > audit-$(date +%F).json
```

<Aside>
For SOC 2 / ISO 27001, ship stderr to an immutable log store and capture the JSON nightly. The in-memory buffer alone is not auditable evidence.
</Aside>

See also: [Security hardening](/operations/security-hardening/).
