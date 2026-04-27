---
title: Community Recipes
description: Browse and deploy community-contributed Docker Compose stacks from the deploy wizard.
---

SimpleDeploy ships with curated **bundled templates** for common stacks. For everything else, you can browse **community recipes**, Docker Compose stacks contributed by the community.

## How to use

1. Open the deploy wizard.
2. Click **Browse community recipes**.
3. Pick a recipe; review its README in the detail view.
4. Click **Use Recipe** to load the compose into the editor.
5. Edit env vars or domain as needed and deploy as usual.

Community recipes are paste-ready: they include `simpledeploy.*` labels for endpoints, healthchecks, and resource limits.

## Where they come from

The catalog is hosted at <https://vazra.github.io/simpledeploy-recipes/>. Source: <https://github.com/vazra/simpledeploy-recipes>.

The SimpleDeploy server fetches the catalog on demand and caches it for 10 minutes. If the catalog is unreachable, the previous response is served (stale-while-error fallback).

## Disclaimers

Community recipes are reviewed for schema validity and image resolvability, but **not audited for security**. Read the compose file and README before deploying. Treat third-party recipes the same as third-party code.

## Configuration

The catalog URL can be overridden in `config.yml`:

```yaml
recipes_index_url: https://vazra.github.io/simpledeploy-recipes/index.json
```

Set to a self-hosted catalog if you want to maintain your own recipe registry.
