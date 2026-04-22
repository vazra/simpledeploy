---
title: Debugging with an isolated reproduction repo
description: When CI fails but local doesn't, stand up a minimal public GitHub repo that exercises just the broken thing. Often the fastest path to a root cause.
---

When a bug only appears in one environment (CI passes locally, or the opposite), resist the urge to keep patching the full system. Pull the broken piece into an isolated repo and run it bare. The smaller the surface, the faster the root cause emerges.

## When to reach for this

- A test passes locally and fails in CI (or vice versa) and re-running doesn't help.
- A multi-layer stack (compose + proxy + deployer + UI) flakes, and you can't tell which layer is lying.
- You suspect an environment-specific issue (runner speed, kernel, Docker version) but can't prove it.
- A bug fix worked in dev but regressions hit CI only — you need a standing regression test outside the main pipeline.

## Pattern: minimal public repo + CI matrix

1. **Render the broken artifact verbatim**, stripping everything the main system adds. For SimpleDeploy templates the tooling is `e2e/tools/render-template.js` which emits a standalone compose file with `simpledeploy.*` labels removed and endpoint ports mapped to the host. For other subsystems, write a tiny script that does the equivalent: capture the exact input the broken code receives.
2. **Build a matrix of variants** that isolates each hypothesized cause. One cell per hypothesis. Keep the cells as close to minimal-and-identical as possible so only the hypothesis variable differs.
3. **Push to a public repo** with a GitHub Actions workflow that exercises each cell in parallel and dumps logs on failure. Public repos get free runners and are easy to share with collaborators.
4. **Compare cell outcomes**. The cell that fails tells you which piece of the original system is responsible.

## Example: `vazra/tpl-ci-repro` (Nov 2026)

Context: SimpleDeploy's templates e2e was marking n8n-postgres and umami-postgres as probe-failing only in CI. Raw `docker compose up` on my Mac worked for both.

Matrix:
- `n8n/` — the rendered compose, as-is
- `n8n-injected/` — the rendered compose PLUS SimpleDeploy's `InjectSharedNetwork` rewrite applied
- `umami/`, `umami-injected/` — same pair for Umami

Workflow probed each cell's endpoint for up to 360s and dumped container logs on failure.

Outcome:
- `n8n`: reachable in 9s
- `umami`: reachable in 15s
- `n8n-injected`: UNREACHABLE, logs showed `getaddrinfo EAI_AGAIN db`
- `umami-injected`: UNREACHABLE, same pattern

Root cause isolated to the injection rewrite in a single 8-minute CI run. Fix + regression test landed in #9.

## Checklist

- [ ] The reproduction is minimal. If a variable doesn't affect the outcome, strip it.
- [ ] Each matrix cell isolates exactly one hypothesis.
- [ ] Logs from failing cells are dumped automatically (don't rely on the operator remembering to fetch them).
- [ ] The repo is kept around long enough to serve as a regression test — until the fix has survived a release cycle. Then delete it.
- [ ] Findings are written up in `docs/architecture/` or the relevant area doc, linking back to the PR that fixed the root cause.

Template composes, API payloads, migration SQL, Go programs calling a single function — anything that can be captured as a small input can be isolated this way.
