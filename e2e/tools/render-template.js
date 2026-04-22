#!/usr/bin/env node
// Render a template's compose JSON to a standalone docker-compose.yml
// (JSON is valid YAML) for LOCAL reachability testing outside SimpleDeploy.
// Strips simpledeploy.* labels and maps each endpoint to a host port so you
// can curl it directly.
//
// Usage: node render-template.js <template-id> <out-path>
// Prints JSON to stdout: { id, slug, vars, endpoints: [{service,domain,hostPort,containerPort}] }

import { writeFileSync } from 'fs';
import { appTemplates, applyVars, generateSecret } from '../../ui/src/lib/appTemplates.js';

const id = process.argv[2];
const outPath = process.argv[3];
if (!id || !outPath) {
  console.error('usage: render-template.js <template-id> <out-path>');
  process.exit(2);
}

const tpl = appTemplates.find((t) => t.id === id);
if (!tpl) {
  console.error(`unknown template: ${id}`);
  process.exit(2);
}

const slug = `e2e-tpl-${id}`.slice(0, 40).replace(/[^a-z0-9-]/g, '-');

const vars = {};
for (const v of tpl.variables || []) {
  if (v.generate) {
    vars[v.key] = generateSecret(v.generate.length, v.generate.charset);
    continue;
  }
  if (v.default != null) {
    vars[v.key] = v.default;
    continue;
  }
  switch (v.type) {
    case 'domain': vars[v.key] = `${slug}.local`; break;
    case 'email':  vars[v.key] = 'e2e@example.com'; break;
    case 'number': vars[v.key] = String(v.placeholder ?? 8080); break;
    case 'secret': vars[v.key] = generateSecret(24); break;
    case 'enum':   vars[v.key] = (v.options || [])[0]?.value ?? ''; break;
    default:       vars[v.key] = `e2e-${v.key}`;
  }
}

const rendered = applyVars(tpl.compose, vars);

let hostPortCounter = parseInt(process.env.PORT_BASE || '18080', 10);
const endpoints = [];
for (const [svcName, svc] of Object.entries(rendered.services || {})) {
  if (!svc || !svc.labels) continue;
  const epPorts = {};
  const epDomains = {};
  for (const [k, val] of Object.entries(svc.labels)) {
    const m = k.match(/^simpledeploy\.endpoints\.(\d+)\.(port|domain)$/);
    if (m) (m[2] === 'port' ? epPorts : epDomains)[m[1]] = val;
  }
  for (const k of Object.keys(svc.labels)) {
    if (k.startsWith('simpledeploy.')) delete svc.labels[k];
  }
  if (Object.keys(svc.labels).length === 0) delete svc.labels;

  for (const idx of Object.keys(epPorts)) {
    const containerPort = String(epPorts[idx]);
    const domain = epDomains[idx];
    const hostPort = hostPortCounter++;
    svc.ports = Array.isArray(svc.ports) ? svc.ports : [];
    svc.ports.push(`${hostPort}:${containerPort}`);
    endpoints.push({ service: svcName, domain, hostPort, containerPort });
  }
}

writeFileSync(outPath, JSON.stringify(rendered, null, 2));
console.log(JSON.stringify({ id, slug, vars, endpoints }, null, 2));
