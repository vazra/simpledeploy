import { defineConfig } from '@playwright/test';

// Slow specs skipped when E2E_LITE=1. Each pulls large images, spawns
// auxiliary containers (MinIO, DB engines, local registry), or waits on
// long timing windows (webhook delivery). Trims ~15 min from the suite.
const LITE_SKIP = [
  '**/13b-webhook-formats.spec.js',
  '**/27-backup-s3.spec.js',
  '**/28-db-strategies.spec.js',
  '**/29b-private-registry.spec.js',
  '**/slow-*.spec.js',
];

// Template deploy-all matrix. Expensive (pulls ~20 images across
// multi-service stacks). Never runs under lite or full; only when
// E2E_TEMPLATES=1 (e.g. templates changed). See `make e2e-templates`.
const TEMPLATES_SPEC = '**/templates-deploy-all.spec.js';
const TEMPLATES_ONLY = process.env.E2E_TEMPLATES === '1';

export default defineConfig({
  testDir: './tests',
  // In templates-only mode, restrict to admin setup + the deploy-all spec.
  testMatch: TEMPLATES_ONLY
    ? ['**/01-setup.spec.js', TEMPLATES_SPEC]
    : undefined,
  testIgnore: TEMPLATES_ONLY
    ? undefined
    : [TEMPLATES_SPEC, ...(process.env.E2E_LITE === '1' ? LITE_SKIP : [])],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['html', { open: 'never' }], ['list']],
  // 240s covers the worst case: multi-service deploy where docker compose
  // pull has to fetch multiple images (the mirror is fast but not free).
  // Deploys expect `Deployed` for up to 180s, so the test timeout must be
  // strictly larger or it masks the real wait.
  timeout: 240_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: `http://localhost:${process.env.SIMPLEDEPLOY_PORT || 8500}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
  globalSetup: './global-setup.js',
  globalTeardown: './global-teardown.js',
});
