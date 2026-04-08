# Versioning, Packaging & Publishing

## Overview

Add automated versioning, cross-platform builds, and package distribution to simpledeploy. Users can install via Homebrew (macOS) or APT (Ubuntu/Debian) with auto-updates.

## Versioning

- Conventional commits (feat:, fix:, chore:, etc.)
- Release Please GitHub Action on main branch
- Auto-creates release PRs that bump CHANGELOG.md
- Merging a release PR creates a git tag (vX.Y.Z)
- No version file in repo; version injected at build time via `-ldflags -X main.version=...`
- CLI exposes `simpledeploy version` command printing version, commit, build date

## Build & Publish (GoReleaser)

Triggered on tag push (from merged release PR).

### Targets

| OS    | Arch  | Format        |
|-------|-------|---------------|
| Linux | amd64 | tar.gz, .deb  |
| Linux | arm64 | tar.gz, .deb  |
| macOS | amd64 | tar.gz        |
| macOS | arm64 | tar.gz        |

### Build steps

1. CI checks out code
2. Sets up Node.js, runs `make ui-build`
3. Sets up Go
4. Runs GoReleaser, which:
   - Builds binaries with ldflags (version, commit, date)
   - Creates tar.gz archives
   - Creates .deb packages (Linux)
   - Generates checksums (SHA256)
   - Uploads everything to GitHub Releases
   - Pushes Homebrew formula to `vazra/homebrew-tap`

### ldflags

```
-X main.version={{.Version}}
-X main.commit={{.ShortCommit}}
-X main.date={{.Date}}
```

## Homebrew (macOS)

- Tap repo: `vazra/homebrew-tap` on GitHub
- GoReleaser auto-generates and pushes formula
- Install: `brew install vazra/tap/simpledeploy`
- Updates: `brew upgrade simpledeploy`

## APT Repository (Ubuntu/Debian)

### Hosting

- Repo: `vazra/apt-repo` on GitHub with GitHub Pages enabled
- Serves signed APT repository at `https://vazra.github.io/apt-repo`

### Repo structure

```
vazra/apt-repo/
  gpg.key              # Public GPG key (armored)
  dists/
    stable/
      main/
        binary-amd64/
          Packages
          Packages.gz
        binary-arm64/
          Packages
          Packages.gz
      Release
      Release.gpg
      InRelease
  pool/
    main/
      s/
        simpledeploy/
          simpledeploy_1.0.0_amd64.deb
          simpledeploy_1.0.0_arm64.deb
```

### GPG signing

- Generate a dedicated GPG key for repo signing
- Private key stored as GitHub Actions secret (`APT_GPG_PRIVATE_KEY`)
- Public key published at `https://vazra.github.io/apt-repo/gpg.key`

### Update flow (in release workflow)

1. GoReleaser produces .deb files
2. Release job clones `vazra/apt-repo`
3. Copies new .deb files to `pool/main/s/simpledeploy/`
4. Regenerates `Packages`, `Packages.gz` with `dpkg-scanpackages`
5. Generates `Release` file with `apt-ftparchive`
6. Signs: `gpg --detach-sign -a -o Release.gpg Release` and `gpg --clearsign -o InRelease Release`
7. Commits and pushes to `vazra/apt-repo` (triggers GitHub Pages deploy)

### User install

```bash
# Add GPG key
curl -fsSL https://vazra.github.io/apt-repo/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/vazra.gpg

# Add repo
echo "deb [signed-by=/usr/share/keyrings/vazra.gpg arch=amd64] https://vazra.github.io/apt-repo stable main" | sudo tee /etc/apt/sources.list.d/vazra.list

# Install
sudo apt update && sudo apt install simpledeploy
```

Auto-updates via `apt update && apt upgrade`.

## CI Workflows

### ci.yml (push/PR)

- Lint with golangci-lint
- Run tests (`go test ./...`)
- Build check (`make build`)
- Runs on: push to any branch, PRs to main

### release.yml (main only)

- Release Please step: creates/updates release PR or creates tag on merge
- On tag creation:
  - Build UI assets
  - Run GoReleaser (builds, packages, publishes to GitHub Releases + Homebrew tap)
  - Update APT repo

## Files to Add/Modify

### New files

- `.goreleaser.yml` - build matrix, archive, .deb, brew config
- `.github/workflows/ci.yml` - lint, test, build
- `.github/workflows/release.yml` - release-please + goreleaser + apt update

### Modified files

- `Makefile` - add VERSION ldflags support
- `cmd/simpledeploy/main.go` - add version vars + `version` subcommand

### External repos to create

- `vazra/homebrew-tap` - initialized with README, formula auto-managed
- `vazra/apt-repo` - initialized with GitHub Pages, GPG key, empty repo structure

## Secrets Required

| Secret | Purpose |
|--------|---------|
| `HOMEBREW_TAP_TOKEN` | PAT with repo scope for pushing to vazra/homebrew-tap |
| `APT_REPO_TOKEN` | PAT with repo scope for pushing to vazra/apt-repo |
| `APT_GPG_PRIVATE_KEY` | GPG private key for signing APT repo |
