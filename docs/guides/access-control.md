---
title: IP access control
description: Restrict which client IPs can reach an app with simpledeploy.access.allow. Allowlist-only, default-allow when empty, 404 for blocked IPs.
---

Restrict which client IPs reach an app with the `simpledeploy.access.allow` label. Useful for admin panels, internal APIs, geo-pinning to known office IPs, or staging environments.

## Semantics

| State | Behavior |
|-------|----------|
| Label absent or empty | All traffic allowed (default-open) |
| Label set | **Only** listed IPs/CIDRs allowed (allowlist), everyone else gets `404 Not Found` |

The proxy returns `404` rather than `403` so you don't leak the existence of the app to scanners.

## Examples

### Office + admin laptop

```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.access.allow: "10.0.0.0/8,203.0.113.5"
```

Allows the entire `10.0.0.0/8` private range plus a single public IP.

### Internal-only API

```yaml
labels:
  simpledeploy.endpoints.0.domain: "api.internal.example.com"
  simpledeploy.endpoints.0.port: "8080"
  simpledeploy.access.allow: "10.0.0.0/8,172.16.0.0/12"
```

### Cloudflare in front

If you front the app with Cloudflare, allowlist its IP ranges so only Cloudflare-routed traffic reaches you. Combine with [`trusted_proxies`](/guides/load-balancer/) so the real client IP is used for downstream rate limiting.

## Update without redeploy

```bash
curl -X PUT https://manage.example.com/api/apps/myapp/access \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"allow": "10.0.0.0/8,203.0.113.5"}'
```

Or edit it in the UI: app page, Endpoints tab, **Access control** field.

<Aside>
This is network-layer filtering. It does not replace app-level auth. Stack both: allowlist + login (or API key) + [rate limit](/guides/rate-limiting/).
</Aside>

See also: [Behind a load balancer](/guides/load-balancer/), [Users and roles](/guides/users-roles/), [Compose labels](/reference/compose-labels/).
