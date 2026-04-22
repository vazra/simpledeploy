---
title: Compose injection (InjectSharedNetwork)
description: How SimpleDeploy rewrites user compose files at deploy time, and the default-network pitfall that broke multi-service templates.
---

SimpleDeploy rewrites every user-supplied compose file before handing it to `docker compose up`. The rewrite is done by `InjectSharedNetwork` in [/internal/compose/inject.go](https://github.com/vazra/simpledeploy/blob/main/internal/compose/inject.go).

## What the rewrite does

For every service that has a `simpledeploy.endpoints.N.*` label:

1. Append `simpledeploy-public` to the service's `networks:` list. That is the bridge network the host-native Caddy proxy joins services on so it can reach them by container IP (see [proxy docs](./proxy.md)).
2. Ensure the top-level `networks:` block declares `simpledeploy-public` as `{external: true, name: simpledeploy-public}` (the actual network is created once by `EnsureNetwork` in `/internal/docker/`).

Services without endpoint labels (databases, workers, caches) are left untouched.

## The default-network pitfall

Declaring `networks:` on a service in Compose **removes the implicit attachment to the project's default bridge**. This is a Compose spec behavior, not a Docker one.

So when `InjectSharedNetwork` adds `networks: [simpledeploy-public]` to an app service, the app *loses* its connection to every sibling service that was relying on the implicit default network. Databases and redis instances in particular go unreachable by DNS name (`getaddrinfo EAI_AGAIN db`).

The fix (landed in #9): when SimpleDeploy *creates* a `networks:` block on a service (the common case, since templates don't declare one), it materializes the list as `[default, simpledeploy-public]`. The explicit `default` preserves the project-default attachment; the shared network is added on top.

If the user's compose already declares an explicit `networks:` block on a service, SimpleDeploy trusts that declaration and only appends `simpledeploy-public` to it. The user is responsible for keeping `default` (or their chosen sibling-reachable network) in the list.

## Why this was hard to catch

The bug only manifests when **all three** conditions are true:

1. The endpoint-bearing service has no explicit `networks:` block in its compose.
2. The endpoint service depends on at least one sibling service (e.g. `depends_on: db`) reached by service-name DNS.
3. The sibling service has no endpoint label and therefore no `networks:` block gets synthesized for it.

Templates like `nginx-static` (single service) and `mailpit` (single service, two endpoints on the same container) never triggered it. Templates like `n8n-postgres` and `umami-postgres` (app + db) hit it every time.

A standalone reproduction repo (no SimpleDeploy) made the diagnosis straightforward: rendering the template compose verbatim worked, applying the same injection as SimpleDeploy produced the exact failure. See [isolated-repro debugging](../contributing/isolated-repro.md) for the pattern.

## Regression test

`TestInjectSharedNetworkPreservesDefaultForDepServices` in `/internal/compose/inject_test.go` guards the invariant: any endpoint service that SimpleDeploy materializes `networks:` for must list both `default` and `simpledeploy-public`.

If you add a new compose-rewrite pass, hand-exercise it with a multi-service template (app + db is the minimal shape) and probe the app endpoint end-to-end through the proxy.
