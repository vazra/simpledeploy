---
title: Proxy (embedded Caddy)
description: How buildConfig() composes Caddy JSON, custom modules plug in, and TLS modes map to automation policies.
---

import { Aside } from '@astrojs/starlight/components';

Caddy runs in-process as a library. There is no Caddyfile. All config is JSON, built programmatically and pushed via `caddy.Load()`. Source: [/internal/proxy/](https://github.com/vazra/simpledeploy/tree/main/internal/proxy).

## Lifecycle

1. `NewCaddyProxy(cfg)` allocates the proxy struct (no Caddy started yet).
2. The reconciler calls `SetRoutes(routes)` after every reconcile pass.
3. `SetRoutes` validates every domain against `^[a-zA-Z0-9][a-zA-Z0-9.*-]*$`, registers any per-domain rate-limit and IP-allowlist configs into the package-level registries, then calls `reload()`.
4. `reload()` runs `buildConfig()` to produce the full JSON, marshals it, and calls `caddy.Load(data, true)`. Caddy hot-reloads in-place; in-flight requests complete on the old config, new requests use the new one.

`Stop()` calls `caddy.Stop()`. The admin endpoint is always disabled (`admin.disabled: true`); the only way to change config is via this code path.

## buildConfig

Located in [/internal/proxy/proxy.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/proxy.go), it returns a `map[string]interface{}` shaped like Caddy's JSON schema.

For each `Route`:

- One Caddy route entry with `match.host` set to the route's domain.
- A handler chain in this exact order: `simpledeploy_ipaccess`, `simpledeploy_ratelimit`, `simpledeploy_metrics`, then `reverse_proxy` to `r.Upstream`.
- If TLS mode is `custom`, append a load-files entry pointing at `<app_dir>/certs/<domain>.crt` and `.key` with `tags: [domain]`.

The whole thing is then assembled into a single HTTP server listening on the configured `listenAddr` (typically `:443`).

## TLS automation policies

The `tlsCfg` block changes shape based on `tlsMode`:

| Mode | What buildConfig emits |
|------|------------------------|
| `auto` (with email) | `tls.automation.policies[0].issuers[0]` = `{module: acme, email: <tlsEmail>}`. ACME flow uses HTTP-01 challenge on `:80`. |
| `local` | Same shape, issuer module is `internal` (Caddy's local CA). Storage root is set to `<dataDir>/caddy`. |
| `custom` | No automation policy. Caddy serves whatever was loaded via `load_files`. |
| `off` | `server.automatic_https.disable: true`. HTTP only. |

Modes can mix per route: a single proxy can serve some routes with ACME, others with custom certs, others HTTP-only by setting `simpledeploy.endpoints.N.tls`.

## Custom Caddy modules

All three are registered in `init()` of their respective files in [/internal/proxy/](https://github.com/vazra/simpledeploy/tree/main/internal/proxy). They are real Caddy modules (`http.handlers.simpledeploy_*`), not custom HTTP middleware bolted on the side. This means Caddy's request lifecycle (logging, error handling, response recording) wraps them correctly.

### `simpledeploy_metrics`

[/internal/proxy/reqmetrics.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/reqmetrics.go). Wraps the response writer to capture status code, measures latency from before to after `next.ServeHTTP`, then non-blocking send into `RequestStatsCh` (a package-level `chan<-` set during startup). Dropped if full. Path is normalized via `NormalizePath` (numeric IDs and UUIDs become `{id}`) so the metrics table does not explode in cardinality.

### `simpledeploy_ratelimit`

[/internal/proxy/ratelimit.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/ratelimit.go). Per-domain configs registered via `RateLimiters.Set(domain, cfg)` from `SetRoutes`. Each domain has its own `domainLimiter` keyed by client IP (default), header value, or query param depending on `simpledeploy.ratelimit.by`. Stale buckets are evicted lazily once the per-domain map exceeds 100 entries.

### `simpledeploy_ipaccess`

[/internal/proxy/ipaccess.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/ipaccess.go). Allowlist of IPs and CIDRs from `simpledeploy.access.allow`. Validated as `net.ParseIP` or `net.ParseCIDR` before being added; invalid entries are logged and skipped.

## ACME flow

When `tlsMode = auto`, Caddy obtains certs on first request to a new domain. The HTTP-01 challenge requires port 80 to be reachable from the public Internet for the apex of each registered domain. SimpleDeploy does not bind 80 directly; Caddy's default HTTP server picks it up because Caddy's `automatic_https` is enabled. Issued certs are stored under `<dataDir>/caddy/` and reused across restarts.

<Aside type="note">
For `tlsMode = local`, Caddy's internal CA is self-signed. Browsers will warn unless you import the root cert (printed by Caddy on first run, also at `<dataDir>/caddy/pki/authorities/local/root.crt`). Useful for staging but not production.
</Aside>

## Why no Caddyfile

Caddyfile is fine for static config but a poor fit when routes change at runtime in response to file events. JSON is Caddy's native format, fully round-trippable, and `caddy.Load()` is faster than re-parsing a Caddyfile. It also means SimpleDeploy never has to escape user input into a DSL.

## Testing

`MockProxy` in [/internal/proxy/mock.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/mock.go) implements the same interface and just records the routes it was given, so tests can assert on the route table without starting Caddy.
