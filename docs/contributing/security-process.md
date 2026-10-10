---
title: Security process
description: Maintainer runbook for security reviews, private fixes, advisories, CVEs and security releases.
---

This page is for maintainers. Reporters should follow [`SECURITY.md`](https://github.com/vazra/simpledeploy/blob/main/SECURITY.md).

## Principles

- **Fix in private, disclose after the release.** Unfixed issues never go into public issues, pull requests, discussions or commit messages.
- **Describe current behavior.** Code comments, tests, docs, commit messages and advisories say what the software does now. They do not narrate how an older version could be abused.
- **Only the latest minor release is patched** (see `SECURITY.md`). Ship the fix as a release on `main`.
- **Every fix gets a regression test** (Go test, vitest, or E2E for full-stack flows).

## Routine security testing

### When

- Before every minor release.
- After changes to authentication, roles and app access, compose handling, backups and restores, the proxy, git sync, or anything that reads or writes files in app folders.
- After bumping `compose-go`, Caddy or the Docker client, since new upstream options can reach the host.
- At least once a quarter.

### Automated checks

| Check | Where |
|---|---|
| `govulncheck` (blocking; only the unfixable `docker/docker` findings are allowed) | CI `govulncheck` job, or `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` locally |
| Dependency updates | Dependabot PRs (gomod, npm, GitHub Actions) |
| UI dependencies | `cd ui && npm audit --omit=dev` |
| Lint and race detector | `golangci-lint run ./...`, `go test -race ./...` |
| Security E2E | `e2e/tests/20-security-validation.spec.js`, `24-multi-user-isolation.spec.js` (chain after `01-setup.spec.js`, see [E2E tests](/contributing/e2e-tests/)) |

### Review checklist

Use the [threat model](/operations/threat-model/) as the scope. For each area touched, confirm:

- **Routes and roles.** Every new route in `internal/api/server.go` has the right middleware (`authMiddleware`, `appAccessMiddleware`, `mutatingAppMiddleware`, `superAdminMiddleware`), and handlers that take an ID check that the record belongs to the app in the URL.
- **Cross-origin requests.** State-changing endpoints stay behind the `http.CrossOriginProtection` wrapper; WebSockets use the shared upgrader (origin check plus periodic re-authorization).
- **Compose validation.** New compose-spec fields that can reach the host (mounts, devices, namespaces, capabilities, host files, build options) are handled in `internal/compose/validate.go` or `rawrefs.go`, with tests. Every path that starts containers goes through the deployer's check before `compose up`.
- **App folder files.** Reads and writes of `docker-compose.yml`, `.env` and sidecars use `internal/fsutil` (`ReadRegularFile`, `WriteFileAtomic`, `EnsureNoSymlinks`).
- **Outbound requests.** User-supplied URLs (webhooks, S3 endpoints, git remotes) go through the existing address checks and validators.
- **Git sync.** Pulled content only changes what [git sync](/operations/git-sync/) documents; users, roles, registries and access stay dashboard-only.
- **Backups and restores.** Restores target only the app's own containers; archives are validated and size-limited; paths and schedules are validated.
- **Proxy.** One app per domain; per-app IP allowlists and rate limits apply to every host form; certs only for an app's own endpoints.
- **Secrets.** Viewers do not receive secret values; audit entries and logs do not contain secrets.
- **Docs and install commands.** Install instructions point at pinned releases with checksum verification, not at unowned domains.

Record findings in a draft security advisory (below), not in a public issue.

## Handling a vulnerability

### 1. Intake and triage

1. Reports arrive through private vulnerability reporting (an advisory in triage) or `security@vazra.us`. Acknowledge within the times in `SECURITY.md`.
2. Reproduce, then rate severity with a CVSS vector and pick CWE IDs.
3. For an internal finding, open a draft advisory yourself: GitHub **Security > Advisories > New draft security advisory**, or:

   ```bash
   gh api -X POST repos/vazra/simpledeploy/security-advisories --input advisory.json
   ```

   Keep the description to impact, affected versions and workarounds. Fill `vulnerabilities[].vulnerable_version_range` (e.g. `<= 1.4.2`); leave `patched_versions` empty for now.

### 2. Fix in the advisory's private fork

1. On the advisory page choose **Start a temporary private fork** (or `gh api -X POST repos/vazra/simpledeploy/security-advisories/GHSA-xxxx-xxxx-xxxx/forks`).
2. Work in a git worktree based on `origin/main`. Push **only** to the fork, with an explicit URL:

   ```bash
   git push https://github.com/vazra/simpledeploy-ghsa-xxxx-xxxx-xxxx.git my-branch
   ```

   Never `git push` to `origin` while the fix is unreleased; worktree branches created from `origin/main` track `origin`.
3. GitHub Actions do not run in the private fork. Run checks locally: `make test`, `cd ui && npm test`, and the related E2E chain.
4. Get an independent review of the diff before merging.
5. Squash the branch into one commit with a neutral message, for example `fix: security hardening (GHSA-xxxx-xxxx-xxxx)`, and check the tree is unchanged:

   ```bash
   git switch -c squash origin/main && git merge --squash my-branch && git commit
   git diff --stat my-branch squash   # must be empty
   ```

6. Open the PR inside the fork and merge it from the advisory page with **Merge pull request**. There is no API for this step.

### 3. Release

1. The advisory merge lands on `main` as a commit titled "Merge commit from fork", which release-please cannot parse, so no release PR appears. Open a small follow-up PR (for example a docs update) titled `fix: <same summary> (GHSA-xxxx-xxxx-xxxx)` and squash-merge it. Add a `Release-As: x.y.z` footer to the squash commit body to pick the version.
2. Merge the release-please PR and watch the `Release` workflow (`release-please`, `goreleaser`, `update-apt-repo`). Confirm the GitHub release has binaries, `.deb` packages and `checksums.txt`.
3. If the fix changes behavior operators rely on, add an entry to "Breaking Changes on Upgrade" in [security hardening](/operations/security-hardening/) and update the threat model.

### 4. CVE and publication

1. Set the patched version and final wording on the advisory (`patched_versions`, "Fixed in x.y.z").
2. Request a CVE: **Request CVE** on the advisory page, or

   ```bash
   gh api -X POST repos/vazra/simpledeploy/security-advisories/GHSA-xxxx-xxxx-xxxx/cve
   ```

   GitHub reviews the request (usually within a few business days) and adds the CVE ID to the advisory. Requesting before publishing lets the advisory go out with its CVE ID; requesting after publishing also works.
3. Credit the reporter in the advisory's credits unless they asked to stay anonymous.
4. Publish: **Publish advisory**, or `gh api -X PATCH repos/vazra/simpledeploy/security-advisories/GHSA-xxxx-xxxx-xxxx -f state=published`. Publishing removes the temporary private fork.
5. Announce the release and advisory (release notes, Discussions) and tell operators to upgrade.

### 5. Afterwards

- Make sure CI on `main` is green.
- Close the loop with the reporter.
- Add any new review items to the checklist on this page.
