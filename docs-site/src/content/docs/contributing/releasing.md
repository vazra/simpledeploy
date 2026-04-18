---
title: Releasing
description: release-please, goreleaser, and the post-release checklist.
---

SimpleDeploy releases through `release-please` (driven by Conventional Commits) and `goreleaser` (binaries + packages).

## Cadence

When the PR queue contains shippable changes (any `feat:` or `fix:` since the last tag), a release-please PR is open against `main` with a generated CHANGELOG bump. Merging that PR cuts a release.

Patch releases for security fixes can be cut anytime by manually editing the release-please PR's version bump.

## What gets built

`goreleaser` (config in `.goreleaser.yml`) produces:

- Linux AMD64 and ARM64 binaries (tarball).
- macOS AMD64 and ARM64 binaries (tarball).
- Debian package (`.deb`) with a systemd service.
- Homebrew tap update (`vazra/homebrew-tap`).
- APT repo update (`vazra.github.io/apt-repo`).
- GitHub release with all artifacts and the CHANGELOG entry.

Build inputs: Go binary with `-ldflags` for version + commit + date, plus the prebuilt UI bundle from `ui/dist/` copied into `cmd/simpledeploy/ui_dist/`.

## Local release dry-run

```bash
# Validate goreleaser config and run a snapshot build (no publish)
goreleaser release --snapshot --clean
```

## Release-day checklist

1. Skim the release-please PR's CHANGELOG. Reword anything unclear.
2. Confirm `make test` and `make e2e` are green on the PR.
3. Merge the release-please PR. Tag is created automatically.
4. CI runs `goreleaser`. Watch the workflow.
5. Verify the new version appears in: GitHub Releases, Homebrew tap, APT repo.
6. Bump the docs site if any reference is version-pinned.
7. Announce in the Blog (`docs-site/src/content/docs/blog/`) and GitHub Discussions.

## Hotfix

For a hotfix without other queued changes:

1. Branch from the release tag.
2. Land the fix as `fix:` commit(s) on `main` (or directly on a release branch if main has unreleased work you want to hold back).
3. Let release-please open a patch PR. Merge it.

## Post-mortem releases

For incident-driven security releases, follow [`SECURITY.md`](https://github.com/vazra/simpledeploy/blob/main/SECURITY.md) for coordinated disclosure.
