#!/usr/bin/env node
// Emit unique docker.io-bound image refs referenced by:
//   - ui/src/lib/appTemplates.js
//   - ui/src/lib/serviceTemplates.js
//   - e2e/fixtures/**/*.yml  (compose fixtures for tests)
//
// Output: one image ref per line on stdout, sorted. Non-docker.io refs
// (ghcr.io/..., quay.io/..., registries with "." or ":" in the host) are
// skipped: those are already at non-rate-limited registries, so mirroring
// them buys nothing.
//
// Consumed by .github/workflows/mirror-images.yml to decide what to copy
// into ghcr.io/vazra/<path>:<tag>.

import { readdirSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { appTemplates } from '../../ui/src/lib/appTemplates.js';
import { serviceTemplates } from '../../ui/src/lib/serviceTemplates.js';

const here = dirname(fileURLToPath(import.meta.url));
const fixturesDir = join(here, '..', 'fixtures');

function collectImagesFromObject(node, out) {
  if (node == null || typeof node !== 'object') return;
  if (Array.isArray(node)) {
    for (const item of node) collectImagesFromObject(item, out);
    return;
  }
  for (const [k, v] of Object.entries(node)) {
    if (k === 'image' && typeof v === 'string') out.add(v);
    else collectImagesFromObject(v, out);
  }
}

function collectImagesFromYAML(text, out) {
  // Narrow line-based match; fixtures are hand-written compose files
  // with canonical "    image: <ref>" lines (quoted or not).
  const re = /^\s+image:\s*['"]?([^'"#\s]+)['"]?/gm;
  for (const m of text.matchAll(re)) out.add(m[1]);
}

function walkYAML(dir, out) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name);
    if (entry.isDirectory()) walkYAML(p, out);
    else if (/\.ya?ml$/.test(entry.name)) {
      collectImagesFromYAML(readFileSync(p, 'utf8'), out);
    }
  }
}

function isDockerHubRef(ref) {
  // Only the part before the first "/" can be a registry host. If there
  // is no "/", the ref is a single-segment docker.io/library image
  // (e.g. "nginx:alpine") - Docker Hub.
  const slashIdx = ref.indexOf('/');
  if (slashIdx === -1) return true;
  const first = ref.slice(0, slashIdx);
  if (first === 'localhost') return false;
  if (first.includes('.') || first.includes(':')) {
    return first === 'docker.io';
  }
  return true;
}

function normalizeToDockerHub(ref) {
  // Strip "docker.io/" and "library/" prefixes so the mirror target is
  // consistent regardless of how the ref was written.
  let r = ref.replace(/^docker\.io\//, '');
  r = r.replace(/^library\//, '');
  return r;
}

const images = new Set();
for (const tpl of appTemplates) collectImagesFromObject(tpl.compose, images);
for (const svc of serviceTemplates) {
  if (typeof svc?.config?.image === 'string') images.add(svc.config.image);
}
walkYAML(fixturesDir, images);
images.delete('');

const dockerHub = [...images]
  .filter(isDockerHubRef)
  .map(normalizeToDockerHub);

const unique = [...new Set(dockerHub)].sort();
for (const img of unique) console.log(img);
