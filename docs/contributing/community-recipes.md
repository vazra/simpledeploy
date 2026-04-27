---
title: Contributing a Recipe
description: How to publish a Docker Compose stack to the SimpleDeploy community catalog.
---

To add a recipe to the community catalog, open a PR against [vazra/simpledeploy-recipes](https://github.com/vazra/simpledeploy-recipes).

See the repo's [CONTRIBUTING.md](https://github.com/vazra/simpledeploy-recipes/blob/main/CONTRIBUTING.md) for the recipe format and validation rules.

A recipe directory contains:

- `recipe.yml`, metadata (id, name, category, description, tags, author)
- `compose.yml`, full Docker Compose with `simpledeploy.*` labels
- `README.md`, what the app does, env vars, post-deploy notes
- `screenshot.png`, optional preview image

CI on the recipes repo validates the schema, parses compose, and runs `docker manifest inspect` for every image referenced by a recipe before the PR can merge.
