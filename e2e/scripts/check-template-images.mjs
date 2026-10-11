#!/usr/bin/env node
// Checks that every image referenced by an app template or service template
// resolves in its registry (`docker manifest inspect`, no layer pulls).
// Catches typos and yanked tags. Run by the Templates workflow on template
// changes and weekly; not part of the per-push E2E suite.
//
//   node e2e/scripts/check-template-images.mjs

import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { appTemplates } from '../../ui/src/lib/appTemplates.js';
import { serviceTemplates } from '../../ui/src/lib/serviceTemplates.js';

const execFileP = promisify(execFile);
const CONCURRENCY = 4;
const ATTEMPTS = 4;
// Registry or network hiccups, not a verdict on the image.
const RATE_LIMIT_RE = /toomanyrequests|rate limit/i;
const TRANSIENT_RE = /toomanyrequests|rate limit|timeout|timed out|TLS handshake|connection reset|EOF|50[0-9] |Service Unavailable|Bad Gateway/i;

function collectImages(node, out) {
  if (node == null || typeof node !== 'object') return;
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

async function inspect(img) {
  let last = '';
  for (let attempt = 1; attempt <= ATTEMPTS; attempt++) {
    try {
      await execFileP('docker', ['manifest', 'inspect', img], { timeout: 60_000 });
      return null;
    } catch (err) {
      // A killed process (our timeout) has no stderr.
      last = err?.killed ? 'timeout' : String(err?.stderr || err?.message || err).trim();
      if (!TRANSIENT_RE.test(last)) return last;
      await new Promise((r) => setTimeout(r, 3_000 * attempt));
    }
  }
  return `transient errors on every attempt: ${last}`;
}

const images = allTemplateImages();
if (images.length === 0) {
  console.error('no template images found');
  process.exit(1);
}
const failures = [];
let next = 0;
await Promise.all(
  Array.from({ length: CONCURRENCY }, async () => {
    while (next < images.length) {
      const img = images[next++];
      const err = await inspect(img);
      if (err) failures.push({ img, err });
    }
  }),
);
const real = failures.filter((f) => !RATE_LIMIT_RE.test(f.err));
if (real.length > 0) {
  const lines = real.map((f) => `  - ${f.img}: ${f.err}`).sort().join('\n');
  console.error(`Unresolvable template images (${real.length}/${images.length}):\n${lines}`);
  process.exit(1);
}
if (failures.length > 0) {
  // Registry throttling says nothing about the images; flag it, don't fail.
  console.log(`::warning::${failures.length} template image(s) not checked: registry rate limit`);
  process.exit(0);
}
console.log(`All ${images.length} template images resolve.`);
