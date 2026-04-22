#!/usr/bin/env node
// Pre-pull every image referenced by app templates so that per-test
// `docker compose pull` is a near-no-op. Pulls happen concurrently
// (bounded) and respect SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX when set.
//
// Usage: node e2e/scripts/prepull-templates.mjs [--shard=X/N]
// Shard flag restricts pulls to the images touched by the tests
// executed on that shard (matches Playwright's deterministic split).

import { appTemplates } from '../../ui/src/lib/appTemplates.js';
import { spawn } from 'node:child_process';

const CONCURRENCY = 6;
const MIRROR = process.env.SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX || '';

function parseShard() {
  const arg = process.argv.find((a) => a.startsWith('--shard='));
  if (!arg) return null;
  const [x, n] = arg.slice(8).split('/').map(Number);
  if (!x || !n || x < 1 || x > n) return null;
  return { index: x - 1, total: n };
}

function shardSlice(items, shard) {
  if (!shard) return items;
  const size = Math.ceil(items.length / shard.total);
  return items.slice(shard.index * size, (shard.index + 1) * size);
}

function imagesForTemplate(tpl) {
  const svcs = tpl.compose?.services || {};
  return Object.values(svcs).map((s) => s?.image).filter(Boolean);
}

function rewriteForMirror(img) {
  if (!MIRROR) return img;
  if (img.startsWith(MIRROR)) return img;
  // Matches internal/mirror.rewriteRef: rewrite docker.io refs only,
  // stripping docker.io/ and library/ prefixes.
  const [first, ...restArr] = img.split('/');
  const rest = restArr.join('/');
  const hasSlash = img.includes('/');
  const isHost = first === 'localhost' || /[.:]/.test(first);
  if (hasSlash && isHost) {
    if (first === 'docker.io') return MIRROR + rest.replace(/^library\//, '');
    return img;
  }
  return MIRROR + img;
}

function pull(img) {
  return new Promise((resolve) => {
    const ref = rewriteForMirror(img);
    const t0 = Date.now();
    const p = spawn('docker', ['pull', ref], { stdio: ['ignore', 'ignore', 'pipe'] });
    let stderr = '';
    p.stderr.on('data', (d) => { stderr += d.toString(); });
    p.on('close', (code) => {
      const dt = ((Date.now() - t0) / 1000).toFixed(1);
      if (code === 0) {
        console.log(`[prepull] ok  ${ref} (${dt}s)`);
      } else {
        // Non-fatal: test will fall back to compose pull.
        console.warn(`[prepull] miss ${ref} (${dt}s): ${stderr.trim().split('\n').pop()}`);
      }
      resolve();
    });
  });
}

async function runPool(items, worker) {
  let i = 0;
  const workers = Array.from({ length: CONCURRENCY }, async () => {
    while (i < items.length) {
      const idx = i++;
      await worker(items[idx]);
    }
  });
  await Promise.all(workers);
}

const shard = parseShard();
const tpls = shardSlice(appTemplates, shard);
const images = [...new Set(tpls.flatMap(imagesForTemplate))];

console.log(`[prepull] ${images.length} unique images across ${tpls.length} templates` +
  (shard ? ` (shard ${shard.index + 1}/${shard.total})` : '') +
  (MIRROR ? ` via mirror ${MIRROR}` : ''));

const t0 = Date.now();
await runPool(images, pull);
console.log(`[prepull] done in ${((Date.now() - t0) / 1000).toFixed(1)}s`);
