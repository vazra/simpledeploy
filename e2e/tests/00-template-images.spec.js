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

test.describe('Template image manifests', () => {
  test('every template image resolves via docker manifest inspect', async () => {
    test.setTimeout(120_000);
    const images = allTemplateImages();
    expect(images.length, 'no template images found').toBeGreaterThan(0);

    const results = await Promise.all(
      images.map(async (img) => {
        try {
          await execFileP('docker', ['manifest', 'inspect', img], {
            timeout: 30_000,
          });
          return { img, ok: true };
        } catch (err) {
          return { img, ok: false, err: String(err?.stderr || err?.message || err) };
        }
      }),
    );

    const failures = results.filter((r) => !r.ok);
    if (failures.length > 0) {
      const msg = failures.map((f) => `  - ${f.img}: ${f.err.trim()}`).join('\n');
      throw new Error(`Unresolvable template images:\n${msg}`);
    }
  });
});
