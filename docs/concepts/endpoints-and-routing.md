---
title: Endpoints and routing
description: How endpoints map (domain, service, port, TLS) to Caddy routes inside the embedded proxy.
---

An **endpoint** is a tuple: `(domain, service, port, tls_mode, protocol, path)`. One app can declare multiple endpoints, and several endpoints may share a domain. Each endpoint becomes one Caddy route. Caddy receives all inbound traffic on `:80` and `:443` and matches by `Host` header, plus the optional path and gRPC content-type matchers.

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

For each endpoint, [/internal/proxy/route.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/route.go) (`resolveEndpointUpstream`) picks where Caddy proxies to, in this order:

1. If the target service publishes a host port matching `endpoint.port`, the upstream is `localhost:<host_port>`.
2. Otherwise, the resolver looks up the service's container IP on the `simpledeploy-public` network via the Docker API and uses `<container_ip>:<port>`. Every endpoint-bearing service is auto-attached to this network by the compose mutation step during deploy, so Caddy can reach it whether SimpleDeploy runs natively on Linux (bridge IPs are host-routable) or inside a container on the same network.
3. As a last-ditch fallback when the Docker API lookup fails (daemon down, container not yet attached), the upstream is the Docker DNS name `<service>:<port>`. Only works when Caddy and the target share a network.

You do not need to publish a host port to make an endpoint reachable. Host ports still work and, when present, take precedence (step 1).

<Aside type="caution">
On macOS and Windows with Docker Desktop, container bridge IPs are not routable from the host. A native `./bin/simpledeploy` on macOS cannot reach step 2. Either publish a host port, or run SimpleDeploy itself in a container (see `make dev-docker`).
</Aside>

## TLS modes

Set per endpoint via `simpledeploy.endpoints.N.tls`:

- `auto` (default): Caddy obtains a Let's Encrypt cert via ACME using the email from `tls.email` in your config.
- `local`: Caddy uses its internal CA. Useful for dev/staging without public DNS.
- `custom`: Caddy loads `<app_dir>/certs/<domain>.crt` and `.key` from disk.
- `off`: HTTP only. Caddy's automatic HTTPS is disabled globally when this is the proxy-wide mode.

The proxy-wide mode comes from your config (`tls.mode`). Per-endpoint TLS modes layer on top. See [TLS guide](/simpledeploy/guides/tls/) for the full matrix.

## Protocols and paths

Two optional labels refine an endpoint:

- `simpledeploy.endpoints.N.protocol`: `http` (default), `h2c` or `grpc`. `h2c` and `grpc` proxy to the upstream over HTTP/2 cleartext with immediate flushing. `grpc` additionally only matches native gRPC requests (`Content-Type: application/grpc*`, gRPC-Web excluded).
- `simpledeploy.endpoints.N.path`: a Caddy path matcher such as `/ws*`.

```yaml
labels:
  simpledeploy.endpoints.0.domain: "api.example.com"
  simpledeploy.endpoints.0.port: "50051"
  simpledeploy.endpoints.0.protocol: "grpc"
  simpledeploy.endpoints.1.domain: "api.example.com"
  simpledeploy.endpoints.1.port: "8000"
  simpledeploy.endpoints.1.path: "/ws*"
  simpledeploy.endpoints.2.domain: "api.example.com"
  simpledeploy.endpoints.2.port: "8001"
```

Route order per domain is fixed: `grpc` endpoints, then path endpoints (longest path first), then the catch-all. Every route is terminal, so the first match wins. Endpoints whose matchers collide (same domain, same path, both or neither `grpc`) are rejected at deploy time.

## What Caddy actually gets

Routes are built in [/internal/proxy/proxy.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/proxy.go) (`buildConfig`) and pushed via `caddy.Load()` after every reconcile. Each route gets a 4-handler chain in this order:

1. `simpledeploy_ipaccess` (allowlist match against `simpledeploy.access.allow`)
2. `simpledeploy_ratelimit` (per-domain token bucket, configured via `simpledeploy.ratelimit.*`)
3. `simpledeploy_metrics` (records request count, latency, status code into the `request_stats` table)
4. `reverse_proxy` to the resolved upstream

<Aside type="caution">
Domains are validated against `^[a-zA-Z0-9][a-zA-Z0-9.*-]*$` before being passed to Caddy. Wildcards are allowed (`*.example.com`), but anything fancier is rejected to avoid Caddy config injection.
</Aside>
