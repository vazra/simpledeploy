---
title: Environment variables and secrets
description: Set per-app environment variables via the UI, CLI, or API. Keep secrets out of compose files using a managed .env file.
---

import { Tabs, TabItem, Aside } from '@astrojs/starlight/components';

Three places env vars can come from for an app:

| Source | Where it lives | When to use |
|--------|----------------|-------------|
| Inline `environment:` in compose | `docker-compose.yml` | Non-sensitive defaults, public config |
| `.env` file beside the compose | `apps/myapp/.env`, written by SimpleDeploy | Secrets, per-environment overrides |
| Encrypted registry creds | SQLite, AES-256-GCM | Image pull credentials only, see [Registries](/guides/registries/) |

## Reference vars from compose

Compose interpolates `${VAR}` from the sibling `.env` file at deploy time. Keep secrets out of the compose, reference them from `.env`.

```yaml
services:
  web:
    image: myapp:latest
    environment:
      DATABASE_URL: ${DATABASE_URL}
      LOG_LEVEL: info
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
```

The compose file is safe to commit. `.env` stays on the server.

## Edit env vars

<Tabs>
<TabItem label="UI">
App page, Config tab, **Environment** sub-tab. Add key/value rows, save. Triggers a redeploy automatically.
</TabItem>
<TabItem label="API">
```bash
# Get current
curl https://manage.example.com/api/apps/myapp/env \
  -H "Authorization: Bearer $SD_API_KEY"

# Replace all (full set, not a patch)
curl -X PUT https://manage.example.com/api/apps/myapp/env \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '[
    {"key":"DATABASE_URL","value":"postgres://user:pw@db:5432/app"},
    {"key":"SECRET_KEY","value":"s3cret"}
  ]'
```
</TabItem>
</Tabs>

The PUT writes `apps/myapp/.env` with `0600` permissions.

<Aside type="caution">
Containers don't reload env vars on the fly. After editing, redeploy the app (UI button or `docker compose up -d`) so the new values take effect.
</Aside>

## What not to do

- Don't put secrets directly in `environment:` in the compose if you commit it to Git.
- Don't bake secrets into the image at build time.
- Don't share the same `.env` across staging and prod, scope each context.

See also: [Registries](/guides/registries/) for image pull creds, [REST API](/reference/api/).
