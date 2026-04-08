# Versioning, Packaging & Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add automated versioning (Release Please), cross-platform builds (GoReleaser), Homebrew tap, APT repo, and update docs.

**Architecture:** Release Please creates release PRs from conventional commits. Merging triggers GoReleaser which builds binaries, .deb packages, publishes to GitHub Releases, updates Homebrew tap, and triggers APT repo update via GitHub Pages.

**Tech Stack:** GoReleaser, Release Please (GitHub Action), GitHub Actions, dpkg-scanpackages, apt-ftparchive, GPG signing

---

### Task 1: Add version variables and CLI command

**Files:**
- Modify: `cmd/simpledeploy/main.go` (add version vars + command)

- [ ] **Step 1: Add version variables at top of main.go**

Add after `package main` imports, before `var cfgFile string` (around line 37):

```go
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)
```

- [ ] **Step 2: Add version command**

Add after the existing command vars (after line ~155, before `func init()`):

```go
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("simpledeploy %s (commit: %s, built: %s)\n", version, commit, date)
	},
}
```

- [ ] **Step 3: Register version command in init()**

Add inside `func init()`, with the other `rootCmd.AddCommand` calls:

```go
rootCmd.AddCommand(versionCmd)
```

- [ ] **Step 4: Test manually**

Run: `go run ./cmd/simpledeploy version`
Expected: `simpledeploy dev (commit: unknown, built: unknown)`

- [ ] **Step 5: Commit**

```bash
git add cmd/simpledeploy/main.go
git commit -m "feat(cli): add version command with ldflags support"
```

---

### Task 2: Update Makefile with version ldflags

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update Makefile**

Replace the entire Makefile with:

```makefile
.PHONY: build build-go test clean ui-build

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

ui-build:
	cd ui && npm install && npm run build
	rm -rf cmd/simpledeploy/ui_dist
	cp -r ui/dist cmd/simpledeploy/ui_dist

build: ui-build build-go

build-go:
	go build -ldflags="$(LDFLAGS)" -o bin/simpledeploy ./cmd/simpledeploy

test:
	go test ./...

clean:
	rm -rf bin/ cmd/simpledeploy/ui_dist
```

- [ ] **Step 2: Test build with version**

Run: `make build-go && ./bin/simpledeploy version`
Expected: `simpledeploy dev (commit: <hash>, built: <timestamp>)`

- [ ] **Step 3: Test with explicit version**

Run: `VERSION=1.0.0 make build-go && ./bin/simpledeploy version`
Expected: `simpledeploy 1.0.0 (commit: <hash>, built: <timestamp>)`

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chore: add version ldflags to Makefile"
```

---

### Task 3: Add GoReleaser config

**Files:**
- Create: `.goreleaser.yml`

- [ ] **Step 1: Create .goreleaser.yml**

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: simpledeploy
    main: ./cmd/simpledeploy
    binary: simpledeploy
    env:
      - CGO_ENABLED=1
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.ShortCommit}}
      - -X main.date={{.Date}}
    hooks:
      pre:
        - cmd: echo "UI assets must be pre-built before GoReleaser runs"

archives:
  - id: default
    formats:
      - tar.gz
    name_template: "simpledeploy_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

nfpms:
  - id: deb
    package_name: simpledeploy
    vendor: Vazra
    homepage: https://github.com/vazra/simpledeploy
    maintainer: Vazra <hello@vazra.dev>
    description: Lightweight deployment manager for Docker Compose apps
    license: MIT
    formats:
      - deb
    bindir: /usr/local/bin
    contents:
      - src: ./simpledeploy.service
        dst: /lib/systemd/system/simpledeploy.service
        type: config
    scripts:
      postinstall: ./packaging/postinstall.sh
      preremove: ./packaging/preremove.sh

checksum:
  name_template: "checksums.txt"

release:
  github:
    owner: vazra
    name: simpledeploy

brews:
  - name: simpledeploy
    repository:
      owner: vazra
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: https://github.com/vazra/simpledeploy
    description: Lightweight deployment manager for Docker Compose apps
    license: MIT
    install: |
      bin.install "simpledeploy"
    test: |
      system "#{bin}/simpledeploy", "version"
```

- [ ] **Step 2: Create systemd service file for .deb packaging**

Create `simpledeploy.service` at repo root:

```ini
[Unit]
Description=SimpleDeploy
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/simpledeploy serve --config /etc/simpledeploy/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 3: Create packaging scripts**

Create `packaging/postinstall.sh`:

```bash
#!/bin/sh
set -e
mkdir -p /etc/simpledeploy
mkdir -p /var/lib/simpledeploy
systemctl daemon-reload
echo "SimpleDeploy installed. Run 'simpledeploy init' to generate config."
```

Create `packaging/preremove.sh`:

```bash
#!/bin/sh
set -e
systemctl stop simpledeploy 2>/dev/null || true
systemctl disable simpledeploy 2>/dev/null || true
```

- [ ] **Step 4: Commit**

```bash
git add .goreleaser.yml simpledeploy.service packaging/
git commit -m "chore: add GoReleaser config with deb and brew support"
```

---

### Task 4: Add CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create CI workflow**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test ./...

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make build
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "chore: add CI workflow with lint, test, build"
```

---

### Task 5: Add release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create release workflow**

```yaml
name: Release

on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-please:
    runs-on: ubuntu-latest
    outputs:
      release_created: ${{ steps.release.outputs.release_created }}
      tag_name: ${{ steps.release.outputs.tag_name }}
    steps:
      - uses: googleapis/release-please-action@v4
        id: release
        with:
          release-type: go

  goreleaser:
    needs: release-please
    if: needs.release-please.outputs.release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-node@v4
        with:
          node-version: 20

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build UI
        run: make ui-build

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}

  update-apt-repo:
    needs: [release-please, goreleaser]
    if: needs.release-please.outputs.release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Download .deb files from release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          TAG="${{ needs.release-please.outputs.tag_name }}"
          VERSION="${TAG#v}"
          mkdir -p debs
          gh release download "$TAG" \
            --repo vazra/simpledeploy \
            --pattern "*.deb" \
            --dir debs

      - name: Checkout apt-repo
        uses: actions/checkout@v4
        with:
          repository: vazra/apt-repo
          token: ${{ secrets.APT_REPO_TOKEN }}
          path: apt-repo

      - name: Import GPG key
        run: |
          echo "${{ secrets.APT_GPG_PRIVATE_KEY }}" | gpg --batch --import

      - name: Update APT repository
        run: |
          # Copy debs to pool
          mkdir -p apt-repo/pool/main/s/simpledeploy
          cp debs/*.deb apt-repo/pool/main/s/simpledeploy/

          cd apt-repo

          # Generate Packages for each arch
          for arch in amd64 arm64; do
            mkdir -p dists/stable/main/binary-${arch}
            dpkg-scanpackages --arch ${arch} pool/ > dists/stable/main/binary-${arch}/Packages
            gzip -k -f dists/stable/main/binary-${arch}/Packages
          done

          # Generate Release
          cd dists/stable
          apt-ftparchive release . > Release

          # Sign
          GPG_KEY_ID=$(gpg --list-keys --with-colons | grep '^pub' | head -1 | cut -d: -f5)
          gpg --batch --yes --default-key "$GPG_KEY_ID" --detach-sign -a -o Release.gpg Release
          gpg --batch --yes --default-key "$GPG_KEY_ID" --clearsign -o InRelease Release

      - name: Push apt-repo
        run: |
          cd apt-repo
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add -A
          git commit -m "update: simpledeploy ${{ needs.release-please.outputs.tag_name }}"
          git push
```

- [ ] **Step 2: Create release-please config**

Create `release-please-config.json`:

```json
{
  "packages": {
    ".": {
      "release-type": "go",
      "bump-minor-pre-major": true,
      "bump-patch-for-minor-pre-major": true
    }
  }
}
```

Create `.release-please-manifest.json`:

```json
{
  ".": "0.0.0"
}
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml release-please-config.json .release-please-manifest.json
git commit -m "chore: add release workflow with Release Please, GoReleaser, APT repo"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `docs/deployment.md`

- [ ] **Step 1: Update Installation section in docs/deployment.md**

Replace the `## Installation` and `### Build from Source` sections (lines 10-19) with:

```markdown
## Installation

### macOS (Homebrew)

```bash
brew install vazra/tap/simpledeploy
```

### Ubuntu/Debian (APT)

```bash
# Add GPG key
curl -fsSL https://vazra.github.io/apt-repo/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/vazra.gpg

# Add repository
echo "deb [signed-by=/usr/share/keyrings/vazra.gpg arch=$(dpkg --print-architecture)] https://vazra.github.io/apt-repo stable main" | sudo tee /etc/apt/sources.list.d/vazra.list

# Install
sudo apt update && sudo apt install simpledeploy
```

Updates arrive via `apt update && apt upgrade`.

### Linux (manual download)

```bash
# Download latest release (replace amd64 with arm64 if needed)
curl -L https://github.com/vazra/simpledeploy/releases/latest/download/simpledeploy_linux_amd64.tar.gz | tar xz
sudo mv simpledeploy /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/vazra/simpledeploy.git
cd simpledeploy
make build
sudo cp bin/simpledeploy /usr/local/bin/
```

Requires Go 1.22+ and Node.js 18+.

### Verify Installation

```bash
simpledeploy version
```
```

- [ ] **Step 2: Commit**

```bash
git add docs/deployment.md
git commit -m "docs: add install instructions for brew, apt, and binary download"
```

---

### Task 7: Add .gitignore entries and final cleanup

**Files:**
- Modify: `.gitignore` (or create if missing)

- [ ] **Step 1: Add GoReleaser dist to .gitignore**

Append to `.gitignore`:

```
dist/
```

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: ignore goreleaser dist directory"
```

---

## Setup Notes (not automated, for the user)

After merging these changes, the following manual setup is needed:

1. **Create GitHub repo** `vazra/simpledeploy` (if not done) and push
2. **Create `vazra/homebrew-tap`** repo on GitHub with a README
3. **Create `vazra/apt-repo`** repo on GitHub:
   - Enable GitHub Pages (deploy from main branch)
   - Generate a GPG key: `gpg --full-generate-key` (RSA 4096, no expiry)
   - Export public key: `gpg --armor --export <KEY_ID> > gpg.key`
   - Commit `gpg.key` to the apt-repo root
   - Create initial directory structure: `dists/stable/main/binary-amd64/`, `dists/stable/main/binary-arm64/`, `pool/main/s/simpledeploy/`
4. **Add GitHub secrets** to `vazra/simpledeploy`:
   - `HOMEBREW_TAP_TOKEN`: PAT with `repo` scope for `vazra/homebrew-tap`
   - `APT_REPO_TOKEN`: PAT with `repo` scope for `vazra/apt-repo`
   - `APT_GPG_PRIVATE_KEY`: output of `gpg --armor --export-secret-keys <KEY_ID>`
5. **First release**: push a conventional commit to main, merge the release PR that Release Please creates
