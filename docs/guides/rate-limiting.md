---
title: Rate limiting
description: Per-app rate limits via compose labels. Limit by IP, header value, or path. Returns 429 with Retry-After when tripped.
---

import { Tabs, TabItem, Aside } from '@astrojs/starlight/components';

Rate limiting is per-app, configured with `simpledeploy.ratelimit.*` labels on any service in the compose file.

## Labels

| Label | Default | Description |
|-------|---------|-------------|
| `simpledeploy.ratelimit.requests` | `200` (server config) | Allowed requests per window |
| `simpledeploy.ratelimit.window` | `60s` | Length of the window |
| `simpledeploy.ratelimit.burst` | `50` | Extra short-spike allowance over the steady rate |
| `simpledeploy.ratelimit.by` | `ip` | Key to bucket by: `ip`, `header:NAME`, or `path` |

`requests`/`window` define the steady rate. `burst` lets a client briefly exceed it before getting throttled. Once the bucket is empty, clients get `429 Too Many Requests` with a `Retry-After: 60` header.

## Per-IP (most common)

```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.ratelimit.requests: "100"
      simpledeploy.ratelimit.window: "60s"
      simpledeploy.ratelimit.burst: "20"
      simpledeploy.ratelimit.by: "ip"
```

100 req/min/IP, with burst up to 120.

## Per API key (header)

```yaml
labels:
  simpledeploy.ratelimit.requests: "1000"
  simpledeploy.ratelimit.window: "60s"
  simpledeploy.ratelimit.by: "header:X-API-Key"
```

Each unique `X-API-Key` value gets its own bucket. Requests with no header share one bucket.

## Per-path (cheap fairness)

```yaml
labels:
  simpledeploy.ratelimit.requests: "10"
  simpledeploy.ratelimit.window: "10s"
  simpledeploy.ratelimit.by: "path"
```

Useful for hot endpoints like `/login` or `/search` where you want to keep one path from starving others.

<Aside>
Rate limits are app-wide. There is no per-endpoint label. All endpoints on an app share one limiter keyed on the bare domain.
</Aside>

<Aside type="caution">
The `ratelimit:` block in `config.yaml` controls the **management API** (login throttling), not proxy traffic. Proxy 429s come only from these compose labels.
</Aside>

## Pair with access control

For admin or internal-only paths, put the app behind an IP allowlist with [IP access control](/guides/access-control/) and skip rate limiting entirely.

See also: [Compose labels](/reference/compose-labels/), [Configuration](/reference/configuration/).
