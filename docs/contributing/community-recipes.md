---
title: Contributing a Recipe
description: How to publish a Docker Compose stack to the SimpleDeploy community catalog.
---

Recipes live in [vazra/simpledeploy-recipes](https://github.com/vazra/simpledeploy-recipes), a separate public repo. Anyone can submit one via PR. CI validates schema, parses compose, and resolves every image manifest before a maintainer reviews. Once merged, your recipe appears in every SimpleDeploy server's deploy wizard within minutes.

## Anatomy of a recipe

A recipe is a folder under `recipes/<slug>/` with four files:

```
recipes/<slug>/
  recipe.yml       metadata
  compose.yml      paste-ready Docker Compose with simpledeploy labels
  README.md        what it does, env vars, post-deploy notes
  screenshot.png   optional preview image (max ~1MB)
```

The `<slug>` is a stable identifier (lowercase, kebab-case). It must match `recipe.yml`'s `id` field. Once published, never rename a slug; users may have deployed something that references it.

## Step-by-step

```bash
# 1. Fork & clone
gh repo fork vazra/simpledeploy-recipes --clone
cd simpledeploy-recipes
npm install

# 2. Scaffold a recipe folder
mkdir -p recipes/my-app
$EDITOR recipes/my-app/recipe.yml
$EDITOR recipes/my-app/compose.yml
$EDITOR recipes/my-app/README.md

# 3. Validate locally
node scripts/validate.mjs recipes/my-app

# 4. Open a PR
git checkout -b add-my-app
git add recipes/my-app
git commit -m "feat: add my-app recipe"
gh pr create --title "Add my-app recipe" --body "..."
```

CI runs schema validation, compose parse, and `docker manifest inspect` on every image. A maintainer reviews and merges; the publish workflow rebuilds the index on `main` and deploys to GitHub Pages.

## `recipe.yml` reference

```yaml
schema_version: 1
id: my-app                   # required, kebab-case, must match folder name
name: My App                 # required, human-readable title (≤80 chars)
icon: "🚀"                   # optional emoji
category: web                # required, see below
description: One-line summary shown on the card.
tags: [analytics, dashboard] # optional, max 10
author: yourgithubhandle     # optional but encouraged
homepage: https://example.com
min_simpledeploy_version: 0.5.0  # optional, used to gate against old binaries
```

**Categories** (pick the closest fit):

`web`, `dev-tools`, `databases`, `storage`, `productivity`, `observability`, `auth`, `mail`, `ci`.

If your app spans categories, choose the user's primary mental model: "what would I search for?" Plausible is `observability`, not `web`.

## `compose.yml` requirements

It must:

- Be a valid Docker Compose file. CI runs `js-yaml` and asserts a non-empty `services:` map.
- Reference images that resolve via `docker manifest inspect`. Avoid `latest`; pin to a specific tag.
- Be paste-ready. A user clicks **Use Recipe**, edits a domain or two, and clicks Deploy. No external setup steps.

It should:

- **Include `simpledeploy.*` labels** for at least one HTTP endpoint. Without these the user gets no public URL.

  ```yaml
  labels:
    simpledeploy.endpoints.0.domain: "${DOMAIN:-example.com}"
    simpledeploy.endpoints.0.port: "8080"
    simpledeploy.endpoints.0.tls: "auto"
  ```

- **Use `${VAR:-default}` env interpolation** for anything the user must change. Use placeholder defaults, never real secrets.
- **Set resource limits** under `deploy.resources.limits` (cpus and memory). Pick caps a low-end VPS can handle.
- **Define healthchecks** for every long-running service so SimpleDeploy can show health status.
- **Use named volumes** for persistence and document them in the README.
- **Set `restart: unless-stopped`** unless there's a reason not to.

It must not:

- Bake real secrets into the file.
- Require host-level mounts (`/var/run/docker.sock`, `/etc`, `/sys`) without a clear, documented reason.
- Run privileged containers without justification in the README.
- Require `network_mode: host` (incompatible with SimpleDeploy's reverse proxy).

## `README.md` template

```markdown
# My App

One-paragraph description: what it is, who it's for, when to use it.

## Variables

- `DOMAIN`: public domain pointing at this server.
- `ADMIN_EMAIL`: receives bootstrap admin invitation.
- `SECRET_KEY`: 32+ random characters; generate with `openssl rand -hex 32`.

## Post-deploy

1. Visit `https://<DOMAIN>`.
2. Sign in with the admin email; check inbox for the invite.
3. Configure ... (anything app-specific).

## Backups

Persistent data lives in the `data` named volume. Recommended backup: a SimpleDeploy "volume" backup config, daily.

## Resources

This recipe runs comfortably with 1 vCPU and 1GB RAM. For >100 active users, raise the memory limit to 2GB.

## Links

- Project: https://...
- Docs: https://...
- License: AGPL-3.0
```

Keep it short. The detail screen renders the README inline, so a wall of text hurts UX. If you have a lot to say, link out.

## Screenshots

Optional. If included:

- 1280x720 or larger, max ~1MB.
- Show the running app's main UI, not the install/setup wizard.
- PNG or compressed JPEG.

## Validation locally

```bash
# One recipe
node scripts/validate.mjs recipes/my-app

# All recipes
node scripts/validate.mjs

# Build the catalog index (catches things validation alone misses)
node scripts/build-index.mjs
```

Validation checks:

- All four required files present.
- `recipe.yml` matches the JSON schema (`schema/recipe.schema.json`).
- `recipe.id` equals the folder name.
- `compose.yml` parses and has a `services:` map.

CI additionally runs `docker manifest inspect` for every image; you can do the same locally:

```bash
docker manifest inspect plausible/analytics:v2.1.0
```

## Image hosting and the GHCR mirror

When your recipe merges, a workflow mirrors every image it references into `ghcr.io/vazra/simpledeploy-mirror/<image>` so SimpleDeploy E2E tests and users behind Docker Hub rate limits can still pull it. You don't need to do anything; the mirror is automatic.

If your recipe references a private image, mirroring will fail and CI will block the merge. Either move the image to a public registry or open an issue to discuss.

## Versioning and updates

Recipes evolve. To update:

- **Bump image tags** in `compose.yml`. Test locally if you can.
- **Edit the README** for any new env vars or steps.
- Open a PR; the same CI gate applies.

There is no per-recipe semver. Users always pull the latest version of a recipe; if you ship a breaking change (e.g. requiring a new env var), call it out clearly in the README.

## What makes a great recipe

- **Solves a complete user need.** "Run Plausible Analytics" beats "Run Postgres" (which the user could have figured out from a template).
- **Sensible defaults.** A user with a domain and 30 seconds gets a working app.
- **Documents the trade-offs.** Memory needs, persistence, backup story, what to do for production.
- **Stays simple.** A recipe with 12 services and 200 lines of compose is hard to review and rarely deploys cleanly. If your stack is that complex, ship it as a single image with internal services or upstream a Helm-style installer.

## License

The recipes repo is MIT-licensed. By submitting a recipe, you agree to license your contribution under MIT. The apps you reference keep their own licenses; cite them in the README.

## Where to ask questions

- Issue on [vazra/simpledeploy-recipes](https://github.com/vazra/simpledeploy-recipes/issues) for catalog-specific questions.
- Issue on [vazra/simpledeploy](https://github.com/vazra/simpledeploy/issues) for SimpleDeploy itself.
