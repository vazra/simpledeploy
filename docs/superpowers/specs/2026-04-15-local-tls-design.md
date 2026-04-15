# Local TLS Mode (`tls: local`)

## Problem

SimpleDeploy on a local network or home server has no good TLS option. `auto` (ACME) requires public DNS + port 443. `custom` requires manually generating certs with openssl. `off` means no HTTPS. Users running SimpleDeploy on a Raspberry Pi, NAS, or home lab want HTTPS with local domains like `myapp.home.lan` without the manual cert hassle.

## Solution

New `tls.mode: "local"` that uses Caddy's built-in internal Certificate Authority. Caddy auto-generates and manages certs for any routed domain, signed by a persistent local root CA. An unauthenticated trust page lets users download and install the root CA on their devices.

This is a hidden feature - not shown in onboarding or promoted in main docs. Users who need it find it in config reference.

## Config

```yaml
tls:
  mode: "local"   # new option alongside auto/custom/off
```

No extra fields. CA storage derived from `data_dir` automatically.

## Components

### 1. Proxy Layer (`internal/proxy/`)

**`CaddyConfig` struct:** `TLSMode` accepts new value `"local"`.

**`buildConfig()` changes:** When `tlsMode == "local"`, TLS automation policy uses Caddy's `internal` issuer:

```json
{
  "automation": {
    "policies": [{
      "issuers": [{
        "module": "internal"
      }]
    }]
  }
}
```

Caddy's storage pointed to `{data_dir}/pki/` so root CA persists across restarts.

**Endpoint-level override:** Routes with explicit `tls: "off"` or `tls: "custom"` still behave as today. Only routes that would normally use ACME get the internal issuer instead. The `"local"` value is also valid at endpoint level (`simpledeploy.endpoints.N.tls: "local"`).

### 2. CA Certificate Access (`internal/api/`)

**`GET /trust`** (management port, e.g., `http://192.168.1.50:8500/trust`)

- Unauthenticated - no login required
- Only available when `tls.mode: "local"`, returns 404 otherwise
- Self-contained HTML page (no SPA, no JS framework dependency)
- Contains:
  - Brief explanation of why this is needed
  - Download button for `ca.crt`
  - Per-platform install instructions: macOS, Windows, Linux, iOS, Android
  - Each platform section is collapsible, showing step-by-step with screenshots/commands

**`GET /api/tls/ca.crt`**

- Unauthenticated when `tls.mode: "local"`, 404 otherwise
- Returns raw PEM file with `Content-Type: application/x-pem-file`
- Reads from Caddy's PKI storage: `{data_dir}/pki/authorities/local/root.crt`

### 3. UI Warning

When an endpoint uses local TLS (explicitly or inherited from server config), show an amber inline warning on the endpoint config card:

> "Local TLS uses a self-signed CA. Browsers will show warnings unless you install the root certificate on each device. [Install instructions](/trust)"

- Link opens `/trust` in new tab
- Amber/yellow style (informational, not error)
- Only shown on endpoints with local TLS, not globally
- No other UI surface mentions local TLS mode

### 4. Route Resolution (`internal/proxy/route.go`)

`ResolveRoutes` already defaults `tls` to `"auto"` when unset. No change needed there. The proxy layer handles mapping `"auto"` to internal issuer when server mode is `"local"`.

Add `"local"` as valid endpoint-level TLS value so users can mix modes (e.g., server is `auto` but one endpoint is `local`).

**Mode resolution per route:**
- Endpoint has explicit `tls` value -> use that value
- Endpoint has no `tls` value -> inherit server-level `tls.mode`
- This means: server `auto` + endpoint `local` = endpoint uses internal issuer. Server `local` + endpoint `off` = endpoint has no TLS.

### 5. Config Validation

In config loading, validate `tls.mode` is one of: `auto`, `custom`, `off`, `local`. Error on unknown values.

Warn at startup if `tls.mode: "local"` is set and `listen_addr` is on a public-facing interface (best-effort check).

## File Changes

| File | Change |
|---|---|
| `internal/config/config.go` | Validate `"local"` as valid TLS mode |
| `internal/proxy/proxy.go` | `buildConfig()`: internal issuer for local mode, Caddy storage path config |
| `internal/proxy/route.go` | Accept `"local"` as valid endpoint TLS value |
| `internal/api/server.go` | Register `/trust` and `/api/tls/ca.crt` routes |
| `internal/api/trust.go` | New file: trust page handler + CA cert download handler |
| `internal/proxy/proxy_test.go` | Test local mode config generation |
| `internal/proxy/route_test.go` | Test local TLS endpoint resolution |
| `internal/api/trust_test.go` | Test trust page + CA download (auth bypass, 404 when not local) |
| `docs/configuration.md` | Document `tls.mode: "local"` in config reference |
| UI endpoint config | Amber warning banner when endpoint uses local TLS |

## What This Does NOT Include

- No automatic device trust (users must manually install CA)
- No DNS server or mDNS (users manage their own local DNS / `/etc/hosts`)
- No UI settings page for switching TLS modes (config file only)
- No migration from other modes (just change the config and restart)
