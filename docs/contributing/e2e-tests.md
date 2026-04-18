---
title: E2E tests
description: How the Playwright suite is wired and how to add new specs.
---

The E2E suite under `e2e/` builds the binary, starts a real server with Docker, deploys actual compose apps, and exercises every UI flow. Run with `make e2e` (~20 min) or `make e2e-lite` (~6-8 min, skips slow specs).

## Layout

```
e2e/
  playwright.config.js     serial, 1 worker, chromium, 2-min timeout
  global-setup.js          builds binary, starts server on random port
  global-teardown.js       kills server, cleans temp dirs
  fixtures/                compose files for test apps (nginx, multi, postgres)
  helpers/
    server.js              server lifecycle (build, start, stop, config gen)
    auth.js                login/logout helpers, test admin credentials
    api.js                 direct API fetch for setup/teardown
  tests/
    01-setup.spec.js       initial admin account creation
    02-login.spec.js       login, logout, wrong password, session redirect
    ... (numbered, run in order)
    19-cleanup.spec.js     remove all apps, verify clean state
```

## How tests work

- Tests run serially in numbered order. State accumulates: setup creates an admin, deploy creates apps, later tests use them.
- Each test gets a fresh browser context (isolated cookies) but the server is shared.
- Server starts with TLS off, very high rate limits, temp data and apps dirs.
- Three fixture apps: `e2e-nginx`, `e2e-multi`, `e2e-postgres`.
- Cleanup tests remove everything at the end.

## Adding a spec

1. Add a numbered file in `e2e/tests/` that fits the suite's order.
2. Use `loginAsAdmin(page)` from `helpers/auth.js` in `beforeEach` for authenticated tests.
3. Use `getState().baseURL` for the server URL. Hash routing: `${baseURL}/#/path`.
4. Wait for `aside` after login (sidebar means dashboard rendered).
5. Use `.first()` on `getByText()` when the text appears in sidebar and main.
6. Scope assertions to `page.locator('main')` to avoid sidebar matches.
7. For modals, scope to `page.getByRole('dialog')`.

## Common pitfalls

- `getByText('foo')` matches substrings, case-sensitive. Use `{ exact: true }` when needed.
- Don't run individual files in isolation; they depend on prior server state.
- Docker image pulls are slow on first run. Deploy test timeout is 180s.
- The `Secure` cookie flag is conditional on TLS mode; tests run with TLS off.

## Running locally

```bash
make e2e           # full suite, headless (~20 min)
make e2e-lite      # skip slow specs (~6-8 min)
make e2e-headed    # full suite, visible browser
make e2e-report    # open HTML report from last run
make e2e-templates # deploy every app template end-to-end (on-demand)
```

Requires Docker daemon, Go, Node.js.

## Template validation

Two layers guard the built-in app and service templates:

- **`00-template-images.spec.js`** (runs in `e2e` and `e2e-lite`): imports `appTemplates.js` and `serviceTemplates.js`, collects every `image:` reference, and runs `docker manifest inspect` on each in parallel. Fails fast when a tag is typo'd, yanked, or private. Takes ~10-30s.
- **`templates-deploy-all.spec.js`** (only runs under `make e2e-templates` / `E2E_TEMPLATES=1`): deploys every app template through the UI wizard, asserts the "Deployed" pill, then deletes the app via API. Expensive (pulls ~20 multi-service stacks), so it is excluded from both `e2e` and `e2e-lite`. The `.github/workflows/templates.yml` workflow triggers it automatically when `appTemplates.js`, `serviceTemplates.js`, or either template spec changes, so it runs once per template-edit PR.

When adding or editing a template, bias toward running `make e2e-templates` locally before opening the PR; the CI workflow runs it too but locally you get faster feedback.
