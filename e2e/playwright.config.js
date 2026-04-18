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

export default defineConfig({
  testDir: './tests',
  testIgnore: process.env.E2E_LITE === '1' ? LITE_SKIP : undefined,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['html', { open: 'never' }], ['list']],
  timeout: 120_000,
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
