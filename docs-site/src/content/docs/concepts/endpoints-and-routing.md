---
title: Endpoints and routing
description: How endpoints map (domain, service, port, TLS) to Caddy routes inside the embedded proxy.
---

import { Aside } from '@astrojs/starlight/components';

An **endpoint** is a tuple: `(domain, service, port, tls_mode)`. One app can declare multiple endpoints. Each endpoint becomes one Caddy route. Caddy receives all inbound traffic on `:80` and `:443` and matches by `Host` header.

## Declaring endpoints

Endpoints are indexed labels on any service in the app. The reconciler reads `simpledeploy.endpoints.N.*` for `N = 0, 1, 2, ...`:

```yaml
services:
  web:
    image: ghcr.io/example/web:1.4
    ports: ["8080"]
    labels:
      simpledeploy.endpoints.0.domain: "app.example.com"
      simpledeploy.endpoints.0.service: "web"
      simpledeploy.endpoints.0.port: "8080"
      simpledeploy.endpoints.0.tls: "auto"

      simpledeploy.endpoints.1.domain: "admin.example.com"
      simpledeploy.endpoints.1.service: "web"
      simpledeploy.endpoints.1.port: "8080"
      simpledeploy.endpoints.1.tls: "auto"
```

Two endpoints, both pointing at the same service, served on two different domains.

## Upstream resolution

For each endpoint, [/internal/proxy/route.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/route.go) (`resolveEndpointUpstream`) picks where Caddy proxies to:

1. If the target service publishes a host port matching `endpoint.port`, the upstream is `localhost:<host_port>`.
2. Otherwise the upstream is the Docker network address `<service>:<port>`.

Most apps only need to declare a container port. Letting Docker pick the host port is fine; the router uses Docker DNS for the in-network case.

## TLS modes

Set per endpoint via `simpledeploy.endpoints.N.tls`:

- `auto` (default): Caddy obtains a Let's Encrypt cert via ACME using the email from `tls.email` in your config.
- `local`: Caddy uses its internal CA. Useful for dev/staging without public DNS.
- `custom`: Caddy loads `<app_dir>/certs/<domain>.crt` and `.key` from disk.
- `off`: HTTP only. Caddy's automatic HTTPS is disabled globally when this is the proxy-wide mode.

The proxy-wide mode comes from your config (`tls.mode`). Per-endpoint TLS modes layer on top. See [TLS guide](/simpledeploy/guides/tls/) for the full matrix.

## What Caddy actually gets

Routes are built in [/internal/proxy/proxy.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/proxy.go) (`buildConfig`) and pushed via `caddy.Load()` after every reconcile. Each route gets a 4-handler chain in this order:

1. `simpledeploy_ipaccess` (allowlist match against `simpledeploy.access.allow`)
2. `simpledeploy_ratelimit` (per-domain token bucket, configured via `simpledeploy.ratelimit.*`)
3. `simpledeploy_metrics` (records request count, latency, status code into the `request_stats` table)
4. `reverse_proxy` to the resolved upstream

<Aside type="caution">
Domains are validated against `^[a-zA-Z0-9][a-zA-Z0-9.*-]*$` before being passed to Caddy. Wildcards are allowed (`*.example.com`), but anything fancier is rejected to avoid Caddy config injection.
</Aside>
