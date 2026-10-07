// Parse and write simpledeploy.endpoints.N.* compose labels. Pure helpers
// used by the visual compose editor. Endpoints are indexed per service.

export const EP_RE = /^simpledeploy\.endpoints\.(\d+)\.(domain|port|tls|protocol|path)$/

const FIELDS = ['domain', 'port', 'tls', 'protocol', 'path']

export function parseEndpointLabels(services = {}) {
  const eps = []
  for (const svcName of Object.keys(services)) {
    const labels = services[svcName]?.labels || {}
    const byIdx = {}
    for (const [k, v] of Object.entries(labels)) {
      const m = k.match(EP_RE)
      if (!m) continue
      const idx = parseInt(m[1])
      if (!byIdx[idx]) byIdx[idx] = { domain: '', port: '', tls: 'letsencrypt', service: svcName }
      byIdx[idx][m[2]] = v
    }
    for (const idx of Object.keys(byIdx).sort((a, b) => a - b)) {
      eps.push(byIdx[idx])
    }
  }
  return eps
}

// Mutates `services` (pass a clone): removes all endpoint labels, then
// writes `eps` back with per-service indexes starting at 0.
export function writeEndpointLabels(services = {}, eps = []) {
  for (const svcName of Object.keys(services)) {
    const labels = services[svcName]?.labels || {}
    for (const k of Object.keys(labels)) {
      if (EP_RE.test(k)) delete labels[k]
    }
  }
  const bySvc = {}
  for (const ep of eps) {
    if (!bySvc[ep.service]) bySvc[ep.service] = []
    bySvc[ep.service].push(ep)
  }
  for (const [svcName, svcEps] of Object.entries(bySvc)) {
    if (!services[svcName]) continue
    if (!services[svcName].labels) services[svcName].labels = {}
    svcEps.forEach((ep, idx) => {
      const prefix = `simpledeploy.endpoints.${idx}`
      for (const f of FIELDS) {
        if (ep[f]) services[svcName].labels[`${prefix}.${f}`] = ep[f]
      }
    })
  }
  return services
}
