// Fast sanity check: every image referenced by an app template or service
// template resolves in its registry via `docker manifest inspect`. Catches
// typos and yanked tags without actually pulling layers. Runs in both
// e2e-lite and the full suite.

import { test, expect } from '@playwright/test';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { appTemplates } from '../../ui/src/lib/appTemplates.js';
import { serviceTemplates } from '../../ui/src/lib/serviceTemplates.js';

const execFileP = promisify(execFile);

function collectImages(node, out) {
  if (node == null) return;
  if (typeof node !== 'object') return;
  if (Array.isArray(node)) {
    for (const item of node) collectImages(item, out);
    return;
  }
  for (const [k, v] of Object.entries(node)) {
    if (k === 'image' && typeof v === 'string') out.add(v);
    else collectImages(v, out);
  }
}

function allTemplateImages() {
  const images = new Set();
  for (const tpl of appTemplates) collectImages(tpl.compose, images);
  for (const svc of serviceTemplates) {
    if (typeof svc?.config?.image === 'string') images.add(svc.config.image);
  }
  images.delete('');
  return [...images].sort();
}

const CONCURRENCY = 4;
const RATE_LIMIT_RE = /toomanyrequests|rate limit/i;

async function inspectOnce(img) {
  await execFileP('docker', ['manifest', 'inspect', img], { timeout: 30_000 });
}

async function inspectWithRetry(img) {
  // Up to 3 attempts with backoff; retry on rate-limit responses so a
  // transient Docker Hub throttle does not fail the whole suite.
  for (let attempt = 1; attempt <= 3; attempt++) {
    try {
      await inspectOnce(img);
      return { img, ok: true };
    } catch (err) {
      const msg = String(err?.stderr || err?.message || err);
      if (attempt < 3 && RATE_LIMIT_RE.test(msg)) {
        await new Promise((r) => setTimeout(r, 2_000 * attempt));
        continue;
      }
      return { img, ok: false, err: msg };
    }
  }
  return { img, ok: false, err: 'exhausted retries' };
}

async function mapPool(items, limit, fn) {
  const results = new Array(items.length);
  let next = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (true) {
      const i = next++;
      if (i >= items.length) return;
      results[i] = await fn(items[i]);
    }
  });
  await Promise.all(workers);
  return results;
}

test.describe('Template image manifests', () => {
  test('every template image resolves via docker manifest inspect', async () => {
    test.setTimeout(300_000);
    const images = allTemplateImages();
    expect(images.length, 'no template images found').toBeGreaterThan(0);

    const results = await mapPool(images, CONCURRENCY, inspectWithRetry);

    const failures = results.filter((r) => !r.ok);
    // If every failure is a Docker Hub rate-limit, don't fail the test
    // (environmental) - just log. Real bad tags still surface.
    const nonRateLimit = failures.filter((f) => !RATE_LIMIT_RE.test(f.err));
    if (failures.length > 0 && nonRateLimit.length === 0) {
      console.warn(
        `[template-images] skipping ${failures.length} image(s) due to Docker Hub rate limits`,
      );
      return;
    }
    if (nonRateLimit.length > 0) {
      const msg = nonRateLimit.map((f) => `  - ${f.img}: ${f.err.trim()}`).join('\n');
      throw new Error(`Unresolvable template images:\n${msg}`);
    }
  });
});
