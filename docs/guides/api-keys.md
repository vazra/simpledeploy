---
title: API keys
description: Create sd_-prefixed API keys for the CLI and automation. Keys inherit the creator's permissions and are shown once.
---

import { Tabs, TabItem, Aside, Steps } from '@astrojs/starlight/components';

API keys authenticate the CLI, CI pipelines, and any HTTP client against the management API. Format: `sd_` followed by 64 hex characters.

## Create a key

<Tabs>
<TabItem label="UI">
<Steps>

1. Profile, **API keys** tab, **Create**.
2. Give it a name (e.g. `ci-deploy`).
3. Copy the displayed key. **It is shown once and never again.**

</Steps>
</TabItem>
<TabItem label="CLI (server-side)">
```bash
sudo simpledeploy apikey create --name ci-deploy --user-id 1
# Output:
# Created: sd_a1b2c3d4...
```
</TabItem>
<TabItem label="API">
```bash
curl -X POST https://manage.example.com/api/apikeys \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"ci-deploy","expires_at":"2026-12-31T23:59:59Z"}'
# {"id":3,"name":"ci-deploy","key":"sd_a1b2c3..."}
```

`expires_at` is optional (omit for keys that never expire). Past dates are
rejected. `GET /api/apikeys` returns `id`, `name`, `created_at`,
`expires_at`, and `last_used_at` so operators can spot stale keys.
</TabItem>
</Tabs>

<Aside type="caution">
The plaintext key is shown exactly once. SimpleDeploy stores only the HMAC-SHA256 hash. Lost it? Revoke and create a new one.
</Aside>

## Scope and permissions

A key inherits the **role and per-app grants of the user who created it** at the moment of creation. There are no per-key scopes. Want a CI key that only touches one app? Create a dedicated user with access to just that app, then issue the key from that user.

## Use it

In the CLI, set up a context so you don't paste the key into every command:

```bash
simpledeploy context add prod \
  --url https://manage.example.com \
  --api-key sd_a1b2c3...

simpledeploy context use prod
simpledeploy list
```

In `curl` or any HTTP client:

```bash
curl https://manage.example.com/api/apps \
  -H "Authorization: Bearer sd_a1b2c3..."
```

## Revoke

```bash
# UI: Profile, API keys, click trash icon
# Or:
curl -X DELETE https://manage.example.com/api/apikeys/3 \
  -H "Authorization: Bearer $SD_API_KEY"
```

Revocation is immediate. Existing requests in flight will continue, the next one is rejected.

## Rotation

There's no auto-rotation. Recommended cadence:

- CI keys: rotate every 90 days, or whenever a teammate leaves.
- Personal keys: rotate when you change machines.

Workflow: create the new key, deploy it to your CI/clients, then revoke the old key.

See also: [Users and roles](/guides/users-roles/), [Remote management](/guides/remote-management/).
