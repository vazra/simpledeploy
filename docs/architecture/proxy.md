---
title: Proxy (embedded Caddy)
description: How buildConfig() composes Caddy JSON, custom modules plug in, and TLS modes map to automation policies.
---

Caddy runs in-process as a library. There is no Caddyfile. All config is JSON, built programmatically and pushed via `caddy.Load()`. Source: [/internal/proxy/](https://github.com/vazra/simpledeploy/tree/main/internal/proxy).

## Lifecycle

1. `NewCaddyProxy(cfg)` allocates the proxy struct (no Caddy started yet).
2. The reconciler calls `ResolveRoutes(app, resolver)` for each app and collects the routes. The resolver (`DockerResolver` in [/internal/proxy/resolver.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/resolver.go)) answers `<service>:<port>` upstreams by looking up the running container's IP on the `simpledeploy-public` network via Docker API; host-port-backed endpoints skip the resolver entirely and resolve to `localhost:<host_port>`.
3. The reconciler calls `SetRoutes(routes)` with the assembled table.
4. `SetRoutes` skips (with a warning) any route whose domain fails `^[a-zA-Z0-9][a-zA-Z0-9.*-]*$`, so one bad label cannot block every app's routes. Each domain is served by one app (`claimDomains`). The app that already serves it keeps it while it still claims it. Otherwise, including after a restart (the owner map starts empty), the app created first wins: the lowest `Route.AppID` (the store ID, set by the reconciler; apps not in the store yet sort last), with the slug as tie-break. Route order plays no part. Other apps' routes on that domain are dropped with a warning. The per-domain rate-limit and IP-allowlist registries are then replaced as a whole (domains lowercased, port and trailing dot stripped), so one app can never clear or override another app's rules, and the config is reloaded. If the load fails, the routes, domain owners and both registries revert to the set Caddy is still running, so rules keep matching the live config. Handlers carry their route's domain in the Caddy JSON and look up that exact key; Host-based lookup (exact first, then wildcards) is only a fallback. The dashboard's config `domain` is not filtered here: the API refuses it as an endpoint domain for users who are not `super_admin` (`checkEndpointDomains` in [/internal/api/domain.go](https://github.com/vazra/simpledeploy/blob/main/internal/api/domain.go)).
5. The reload runs `buildConfig()` to produce the full JSON, marshals it, and calls `caddy.Load(data, true)` unless the JSON and the size/mtime of every referenced custom cert file match the last successful load (route order is deterministic, so no-op redeploys skip the reload). Cert upload/delete call `ForceReload()` to reload unconditionally. Caddy hot-reloads in-place; in-flight requests complete on the old config, new requests use the new one. On Linux each reload binds a new `SO_REUSEPORT` socket and closes the old one, so connections queued on the old socket but not yet accepted are reset (`curl: (35) ... Connection reset by peer`). Set `net.ipv4.tcp_migrate_req=1` (kernel 5.14+) to have the kernel migrate them instead.

`Stop()` calls `caddy.Stop()`. The admin endpoint is always disabled (`admin.disabled: true`); the only way to change config is via this code path.

## buildConfig

Located in [/internal/proxy/proxy.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/proxy.go), it returns a `map[string]interface{}` shaped like Caddy's JSON schema.

For each `Route`:

- Routes are first passed through `orderRoutes` ([/internal/proxy/match.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/match.go)): grouped by domain in first-seen order; inside a domain `grpc` routes, then path routes (longest first), then the catch-all.
- One Caddy route entry per `Route`, `terminal: true`, with matcher from `routeMatcher`: `host`, plus `path` when set, plus for `grpc` a `header` matcher `Content-Type: application/grpc*` and a `not` matcher excluding `application/grpc-web*`.
- A handler chain in this exact order: `simpledeploy_ipaccess`, `simpledeploy_ratelimit`, `simpledeploy_metrics`, `headers`, `timeouts` (HTTP routes only), then `reverse_proxy` to `r.Upstream`. For `h2c`/`grpc` routes `reverse_proxy` gets `transport: {protocol: http, versions: [h2c]}` and `flush_interval: -1`.
- Request limits on every server (`proxy`, `proxy_extra`, `proxy_http`): `max_header_bytes` is 1 MiB (Caddy's own default is 16 KiB, which answers 431 before any handler runs). Caddy's server-wide request body idle timeout is off (`read_idle_timeout: -1`) because it would reset gRPC client-streaming and bidi RPCs whose client stays quiet. HTTP routes get the same protection back from a per-route `timeouts` handler (`read_timeout` 1m): an upload that sends nothing for a minute is cut. `h2c`/`grpc` routes have no body idle limit. Caddy's default 1m write idle timeout stays on; it only applies while a write is in progress, so SSE and long polls are unaffected.
- Caddy drops request header names that contain `_` or `.` before routing, so apps never see them and `simpledeploy.ratelimit.by: header:NAME` only works for names made of letters, digits and `-`.
- If TLS mode is `custom`, append a load-files entry (once per domain) pointing at `<app_dir>/certs/<domain>.crt` and `.key` with `tags: [domain]`.
- When the listener is plain HTTP (`tls.mode: off` and no per-route `local`), the server sets `protocols: [h1, h2, h2c]` so gRPC clients can use prior-knowledge h2c.
- The `proxy` server listens on `listen_addr` with Caddy's default protocols (h1, h2, h3). `extra_listen_addrs` run as a separate `proxy_extra` server with the same routes, TLS connection policies and automatic HTTPS settings, but protocols `h1`/`h2` only (`h1`/`h2`/`h2c` when TLS is off). Certificates are shared through Caddy's cert cache. Keeping HTTP/3 off the extra server stops long-lived h2 streams on that port from holding an old server whose QUIC listeners are already closed, which logged `setting HTTP/3 Alt-Svc header ... no port can be announced` on every reload.

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

[/internal/proxy/reqmetrics.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/reqmetrics.go). Wraps the response writer to capture the final status code (1xx informational responses are skipped, `101` counts), measures latency from before to after `next.ServeHTTP`, then non-blocking send into `RequestStatsCh` (a package-level `chan<-` set during startup). Dropped if full. When the chain returns an error before anything was written, Caddy's error handling writes the response outside this wrapper, so the status comes from the error instead: the `caddyhttp.HandlerError` status (`502` for an unreachable upstream, `504` for a timeout), `499` when the client went away, else `500`. Once a status or body was written, that status is kept even if an error follows. A client disconnect that `reverse_proxy` turns into `WriteHeader(499)` is recorded as `499`. Path is normalized via `NormalizePath` (numeric IDs and UUIDs become `{id}`) so the metrics table does not explode in cardinality.

### `simpledeploy_ratelimit`

[/internal/proxy/ratelimit.go](https://github.com/vazra/simpledeploy/blob/main/internal/proxy/ratelimit.go). Per-domain configs registered via `RateLimiters.Set(domain, cfg)` from `SetRoutes`. Each domain has its own `domainLimiter` keyed by client IP (default), header value, or query param depending on `simpledeploy.ratelimit.by`. Each domain keeps at most 10,000 buckets: when full, buckets idle for a whole window are evicted first, then the least recently used. Keys longer than 128 bytes are stored as their SHA-256.

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
