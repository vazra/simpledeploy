---
title: Community Recipes
description: Browse and deploy community-contributed Docker Compose stacks from the deploy wizard.
---

Community recipes are paste-ready Docker Compose stacks contributed by the community. They sit alongside SimpleDeploy's built-in templates and let you discover and deploy apps that aren't shipped in the binary.

## Bundled templates vs community recipes

| | Bundled templates | Community recipes |
| --- | --- | --- |
| Where it lives | inside the SimpleDeploy binary | public catalog at [github.com/vazra/simpledeploy-recipes](https://github.com/vazra/simpledeploy-recipes) |
| Maintained by | SimpleDeploy maintainers | community contributors |
| Updates | shipped with each SimpleDeploy release | live, no SimpleDeploy upgrade needed |
| Variables | guided form (domain, secrets, scale) | paste-ready compose file you edit before deploy |
| Visible by default | yes, on the wizard's first screen | hidden behind a "Browse community recipes" button |

If a recipe gets popular and stable enough to bundle, it can graduate into the binary. Until then, recipes are the fast path for everything else.

## Browse and deploy a recipe

1. Open the deploy wizard from the dashboard.
2. On the template grid, click **Browse community recipes**.
3. Filter by category or search by name, tag, description.
4. Click any card to open the recipe's README.
5. Click **Use Recipe** to load its `compose.yml` into the wizard's editor.
6. Edit anything you need to (domain, secrets, image tag, ports).
7. Click **Deploy**.

The wizard treats imported recipes like any other compose, so all SimpleDeploy features work: endpoints, TLS, backups, alerts, metrics, logs.

## What to check before deploying

Recipes are community-contributed and reviewed for schema validity and image resolvability, but **not audited for security**. Before deploying, scan the imported compose for:

- **Image source.** Is it from a publisher you trust? Pinned to a specific tag, not `latest`?
- **Volumes.** What does the app persist? Does it need a backup config?
- **Secrets and environment variables.** Defaults like `${ADMIN_PASSWORD:-changeme}` must be replaced.
- **Endpoints.** The recipe's `simpledeploy.endpoints.*` labels point at a placeholder domain. Set yours.
- **Resource limits.** Recipes ship sensible CPU/memory caps. Adjust if your host is small.

Treat a community recipe like any third-party code: skim the README and the compose before clicking Deploy.

## How the catalog works

- The catalog is hosted on GitHub Pages: <https://vazra.github.io/simpledeploy-recipes/>.
- Your SimpleDeploy server fetches the index on demand when you open the browser, then caches it for 10 minutes.
- If the catalog is unreachable, the last successful response is served (stale-while-error). You can still browse what was last fetched.
- Recipe `compose.yml` and `README.md` files are fetched server-side, not by your browser, so corporate networks that block GitHub from end-user devices still work as long as the SimpleDeploy server itself can reach `vazra.github.io`.

### Air-gapped or self-hosted catalog

To run an internal catalog, mirror the public repo (or fork it) and serve `dist/` from any HTTPS host. Then point SimpleDeploy at it:

```yaml
# config.yml
recipes_index_url: https://recipes.internal.example.com/index.json
```

Restart SimpleDeploy. Browse community recipes will pull from your host. The format is documented in the [recipes repo's schemas](https://github.com/vazra/simpledeploy-recipes/tree/main/schema).

To disable community recipes entirely, point the URL at one that 404s. The browser will load with an "unavailable" message; bundled templates remain unaffected.

## Telemetry

When you click **Use Recipe**, the server records a row in the local `recipe_pulls` table with the recipe id and a timestamp. This data:

- never leaves your server,
- is used only to surface a popularity ranking to super_admin users via `/api/recipes/community/popularity`,
- is governed by the audit retention setting (defaults to 365 days).

GitHub does see your server's IP when it fetches the catalog (standard GitHub Pages traffic). It does not see which recipe you chose; the per-recipe file fetches go to the same Pages host as the index.

## Troubleshooting

**"Could not load recipes."** The server failed to reach `vazra.github.io`. Check outbound HTTPS to GitHub from the host. The browser will show the error returned by the server.

**A recipe loads but fails to deploy.** Compose syntax is valid (CI checks this), but your environment may differ: missing env vars, port conflicts, host platform mismatch (e.g. arm64 vs amd64). Read the deploy log; most issues surface there.

**The catalog is stale.** The 10-minute cache may be serving an old index. Restart SimpleDeploy or wait. There's no "refresh" button by design; recipes change rarely and the cache absorbs traffic spikes.

**A recipe I want isn't in the catalog.** Open a PR. See [Contributing a Recipe](/simpledeploy/contributing/community-recipes/).
