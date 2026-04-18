---
title: Writing a Caddy module
description: Register a custom Caddy handler in the simpledeploy process.
---

Caddy is embedded as a library, not a sidecar. To extend its request pipeline, write a Caddy module and register it via `init()` in the proxy package.

## When to add one

- The behavior is request-time and benefits from running inside Caddy (low latency, access to every request).
- Built-in directives cannot express it (or do so awkwardly).
- The data the handler needs comes from SimpleDeploy state (apps, labels, store).

If the behavior is not per-request, do it elsewhere (background loop, deploy-time hook).

## Reference modules

- `simpledeploy_metrics`: records every request into the request stats pipeline.
- `simpledeploy_ratelimit`: per-route token bucket using compose label config.

Both are in `internal/proxy/`.

## Anatomy

```go
package proxy

import "github.com/caddyserver/caddy/v2"

func init() {
    caddy.RegisterModule(MyHandler{})
}

type MyHandler struct {
    // exported fields are JSON-config keys
    Param string `json:"param,omitempty"`
}

func (MyHandler) CaddyModule() caddy.ModuleInfo {
    return caddy.ModuleInfo{
        ID:  "http.handlers.simpledeploy_my",
        New: func() caddy.Module { return new(MyHandler) },
    }
}

// Implement caddyhttp.MiddlewareHandler:
func (h *MyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
    // ... do work, then next.ServeHTTP
    return next.ServeHTTP(w, r)
}

// Optional: caddy.Provisioner (Provision), caddy.Validator (Validate),
// caddy.CleanerUpper (Cleanup) for lifecycle hooks.
```

## Wire into the proxy config

Update `proxy.buildConfig()` to add a route that uses your module by JSON `handler`. Existing modules are good models: the handler appears in a `routes[*].handle[*]` array on each app's route.

## Testing

- Unit-test the handler with `httptest`. Wrap a fake `next` to assert pass-through.
- Integration-test by booting Caddy with a small config that uses your module and hitting it with `http.Client`.

## Constraints

- Allocate sparingly per request. Reuse buffers and structs.
- Do not block on the SimpleDeploy store from the hot path; pre-load config in `Provision` and refresh on Caddy reload.
- Use Caddy's logger (`caddy.Log()`), not `log.Println`.

## Submit

Open a PR with the module, tests, and a sentence in `docs/architecture/proxy.md` describing it.
