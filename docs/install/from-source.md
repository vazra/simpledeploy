---
title: Build from source
description: Build SimpleDeploy from source. Requires Go 1.22+ and Node.js 18+. Recommended for contributors and hackers, not production.
---

<Aside type="caution">
Not recommended for production unless you are patching the codebase. The released binaries are reproducible and signed via CI. Building from source means you also own the upgrade path.
</Aside>

## Prerequisites

- Go 1.22 or newer
- Node.js 18 or newer (with npm or pnpm)
- `make`
- `git`

## Build

<Steps>

1. Clone:

   ```bash
   git clone https://github.com/vazra/simpledeploy.git
   cd simpledeploy
   ```

2. Build the UI and the binary:

   ```bash
   make build
   # output at bin/simpledeploy
   ```

   Need only the Go side (UI already built)?

   ```bash
   make build-go
   ```

3. Install:

   ```bash
   sudo cp bin/simpledeploy /usr/local/bin/
   simpledeploy version
   ```

</Steps>

## Run the test suites

```bash
make test       # Go unit + integration
make e2e        # full Playwright E2E (needs Docker)
```

## Hot reload for development

```bash
make dev        # rebuilds API + UI on change
```

Next: [Set up your dev environment](/contributing/setup/) or jump to [First deploy](/first-deploy/prepare/).
