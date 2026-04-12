<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import AccordionSection from './AccordionSection.svelte'
  import RepeatableField from './RepeatableField.svelte'

  let { compose = {}, slug = '', onchange = () => {}, onerrors = () => {} } = $props()

  // ---- Errors ----
  let errors = $state({})

  // ---- .env file state ----
  let envFileVars = $state([])
  let envFileLoading = $state(false)
  let envFileSaving = $state(false)
  let envFileExpanded = $state(false)

  // ---- Pending env rows (local-only empty rows for Add button) ----
  let pendingEnvRows = $state({})

  onMount(async () => {
    if (!slug) return
    envFileLoading = true
    try {
      const res = await api.getEnv(slug)
      envFileVars = res.data || []
    } catch { /* no env file yet */ }
    envFileLoading = false
  })

  async function saveEnvFile() {
    if (!slug) return
    envFileSaving = true
    try {
      await api.putEnv(slug, envFileVars)
    } catch { /* toast handled by api */ }
    envFileSaving = false
  }

  // ---- Helpers ----
  function deepClone(obj) {
    return JSON.parse(JSON.stringify(obj))
  }

  function setError(key, msg) {
    errors = { ...errors, [key]: msg }
    onerrors(errors)
  }

  function clearError(key) {
    const next = { ...errors }
    delete next[key]
    errors = next
    onerrors(errors)
  }

  function emitChange(updated) {
    onchange(updated)
  }

  // ---- Service names ----
  let serviceNames = $derived(Object.keys(compose.services || {}))
  let firstService = $derived(serviceNames[0] || '')

  // ---- SECTION 1: Endpoints (multi-endpoint) ----
  const SD_PREFIX = 'simpledeploy.'
  const EP_RE = /^simpledeploy\.endpoints\.(\d+)\.(domain|port|tls)$/

  function getLabel(svcName, key) {
    return compose.services?.[svcName]?.labels?.[SD_PREFIX + key] ?? ''
  }

  function setLabel(key, value) {
    if (!firstService) return
    const updated = deepClone(compose)
    if (!updated.services[firstService].labels) {
      updated.services[firstService].labels = {}
    }
    if (value === '' || value == null) {
      delete updated.services[firstService].labels[SD_PREFIX + key]
    } else {
      updated.services[firstService].labels[SD_PREFIX + key] = String(value)
    }
    emitChange(updated)
  }

  function parseEndpoints() {
    const eps = []
    for (const svcName of serviceNames) {
      const labels = compose.services?.[svcName]?.labels || {}
      const byIdx = {}
      for (const [k, v] of Object.entries(labels)) {
        const m = EP_RE.exec(k)
        if (!m) continue
        const idx = parseInt(m[1])
        const field = m[2]
        if (!byIdx[idx]) byIdx[idx] = { domain: '', port: '', tls: 'letsencrypt', service: svcName }
        byIdx[idx][field] = v
        byIdx[idx].service = svcName
      }
      for (const idx of Object.keys(byIdx).sort((a, b) => a - b)) {
        eps.push(byIdx[idx])
      }
    }
    return eps
  }

  let endpoints = $derived(parseEndpoints())

  function updateEndpoint(i, field, value) {
    const eps = parseEndpoints()
    if (i >= eps.length) return
    eps[i] = { ...eps[i], [field]: value }
    writeEndpointsToCompose(eps)
  }

  function addEndpoint() {
    const eps = parseEndpoints()
    eps.push({ domain: '', port: '', tls: 'letsencrypt', service: serviceNames[0] || '' })
    writeEndpointsToCompose(eps)
  }

  function removeEndpoint(i) {
    const eps = parseEndpoints()
    eps.splice(i, 1)
    writeEndpointsToCompose(eps)
  }

  function writeEndpointsToCompose(eps) {
    const updated = deepClone(compose)
    for (const svcName of Object.keys(updated.services || {})) {
      const labels = updated.services[svcName].labels || {}
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
      if (!updated.services[svcName]) continue
      if (!updated.services[svcName].labels) updated.services[svcName].labels = {}
      svcEps.forEach((ep, idx) => {
        const prefix = `simpledeploy.endpoints.${idx}`
        if (ep.domain) updated.services[svcName].labels[prefix + '.domain'] = ep.domain
        if (ep.port) updated.services[svcName].labels[prefix + '.port'] = ep.port
        if (ep.tls) updated.services[svcName].labels[prefix + '.tls'] = ep.tls
      })
    }
    emitChange(updated)
  }

  // ---- Ports parsing/serializing ----
  function parsePorts(svc) {
    const raw = svc?.ports || []
    return raw.map((p) => {
      if (typeof p === 'string') {
        const [host, container] = p.split(':')
        return { host: host ?? '', container: container ?? '' }
      }
      return { host: String(p.published ?? ''), container: String(p.target ?? '') }
    })
  }

  function serializePorts(rows) {
    return rows
      .filter((r) => r.host || r.container)
      .map((r) => `${r.host}:${r.container}`)
  }

  // ---- Environment parsing/serializing ----
  function parseEnv(svc) {
    const raw = svc?.environment
    if (!raw) return []
    if (Array.isArray(raw)) {
      return raw.map((e) => {
        const idx = e.indexOf('=')
        if (idx === -1) return { name: e, value: '' }
        return { name: e.slice(0, idx), value: e.slice(idx + 1) }
      })
    }
    return Object.entries(raw).map(([name, value]) => ({ name, value: value ?? '' }))
  }

  function serializeEnv(rows) {
    const filtered = rows.filter((r) => r.name)
    if (filtered.length === 0) return undefined
    const obj = {}
    for (const r of filtered) obj[r.name] = r.value ?? ''
    return obj
  }

  // ---- Volume parsing/serializing ----
  function parseVolumes(svc) {
    const raw = svc?.volumes || []
    return raw.map((v) => {
      if (typeof v === 'string') {
        const [source, target] = v.split(':')
        return { source: source ?? '', target: target ?? '' }
      }
      return { source: v.source ?? '', target: v.target ?? '' }
    })
  }

  function serializeVolumes(rows) {
    return rows
      .filter((r) => r.source || r.target)
      .map((r) => `${r.source}:${r.target}`)
  }

  // ---- Labels parsing/serializing (non-SD) ----
  function parseLabels(svc) {
    const raw = svc?.labels || {}
    return Object.entries(raw)
      .filter(([k]) => !k.startsWith(SD_PREFIX))
      .map(([key, value]) => ({ key, value: value ?? '' }))
  }

  function serializeLabels(rows, existingLabels) {
    const next = {}
    // preserve simpledeploy.* labels
    for (const [k, v] of Object.entries(existingLabels || {})) {
      if (k.startsWith(SD_PREFIX)) next[k] = v
    }
    for (const r of rows.filter((r) => r.key)) {
      next[r.key] = r.value ?? ''
    }
    return Object.keys(next).length ? next : undefined
  }

  // ---- depends_on parsing (per-dependency conditions) ----
  function parseDependsOn(svc) {
    const raw = svc?.depends_on
    if (!raw) return { services: [], conditions: {} }
    if (Array.isArray(raw)) {
      const conditions = {}
      for (const s of raw) conditions[s] = 'service_started'
      return { services: raw, conditions }
    }
    const services = Object.keys(raw)
    const conditions = {}
    for (const s of services) {
      conditions[s] = raw[s]?.condition ?? 'service_started'
    }
    return { services, conditions }
  }

  function serializeDependsOn(svcs, conditions) {
    if (!svcs.length) return undefined
    // If all conditions are service_started, use simple array format
    const allDefault = svcs.every((s) => (conditions[s] || 'service_started') === 'service_started')
    if (allDefault) return svcs
    const obj = {}
    for (const s of svcs) obj[s] = { condition: conditions[s] || 'service_started' }
    return obj
  }

  // ---- Update service field ----
  function updateService(name, path, value) {
    const updated = deepClone(compose)
    let node = updated.services[name]
    const parts = path.split('.')
    for (let i = 0; i < parts.length - 1; i++) {
      if (node[parts[i]] == null) node[parts[i]] = {}
      node = node[parts[i]]
    }
    const last = parts[parts.length - 1]
    if (value === '' || value == null) {
      delete node[last]
    } else {
      node[last] = value
    }
    emitChange(updated)
  }

  function updateServiceDirect(name, setter) {
    const updated = deepClone(compose)
    setter(updated.services[name])
    emitChange(updated)
  }

  // ---- Rename service ----
  let editingServiceName = $state(null)
  let editingNameValue = $state('')

  function startRename(svcName) {
    editingServiceName = svcName
    editingNameValue = svcName
  }

  function commitRename(oldName) {
    const newName = editingNameValue.trim()
    editingServiceName = null
    if (!newName || newName === oldName) return
    if (compose.services?.[newName]) {
      // duplicate name, abort
      return
    }
    const updated = deepClone(compose)
    // Rename the service key
    const svcData = updated.services[oldName]
    delete updated.services[oldName]
    // Rebuild services preserving order
    const newServices = {}
    for (const [k, v] of Object.entries(compose.services)) {
      if (k === oldName) newServices[newName] = svcData
      else newServices[k] = updated.services[k] || v
    }
    updated.services = newServices
    // Update depends_on references in other services
    for (const [k, svc] of Object.entries(updated.services)) {
      if (!svc.depends_on) continue
      if (Array.isArray(svc.depends_on)) {
        svc.depends_on = svc.depends_on.map((d) => d === oldName ? newName : d)
      } else {
        if (svc.depends_on[oldName]) {
          svc.depends_on[newName] = svc.depends_on[oldName]
          delete svc.depends_on[oldName]
        }
      }
    }
    emitChange(updated)
  }

  // ---- Add service ----
  function addService() {
    const updated = deepClone(compose)
    const baseName = 'service'
    let n = 1
    while (updated.services?.[`${baseName}${n}`]) n++
    if (!updated.services) updated.services = {}
    updated.services[`${baseName}${n}`] = { image: '' }
    emitChange(updated)
  }

  // ---- Top-level networks ----
  function parseNetworks() {
    return Object.entries(compose.networks || {}).map(([name, cfg]) => ({
      name,
      driver: cfg?.driver ?? '',
    }))
  }

  function serializeNetworks(rows) {
    if (!rows.length) return undefined
    const obj = {}
    for (const r of rows.filter((r) => r.name)) {
      obj[r.name] = r.driver ? { driver: r.driver } : {}
    }
    return Object.keys(obj).length ? obj : undefined
  }

  // ---- Top-level volumes ----
  function parseTopVolumes() {
    return Object.entries(compose.volumes || {}).map(([name, cfg]) => ({
      name,
      driver: cfg?.driver ?? '',
    }))
  }

  function serializeTopVolumes(rows) {
    if (!rows.length) return undefined
    const obj = {}
    for (const r of rows.filter((r) => r.name)) {
      obj[r.name] = r.driver ? { driver: r.driver } : {}
    }
    return Object.keys(obj).length ? obj : undefined
  }

  // ---- Env var .env helpers ----
  function isEnvRef(value) {
    return typeof value === 'string' && /^\$\{[A-Za-z_][A-Za-z0-9_]*\}$/.test(value)
  }

  function extractEnvRefKey(value) {
    const m = value.match(/^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$/)
    return m ? m[1] : null
  }

  function resolveEnvRef(value) {
    const key = extractEnvRefKey(value)
    if (!key) return null
    const found = envFileVars.find((v) => v.key === key)
    return found ? found.value : null
  }

  // Confirmation state for move-to-env
  let envConfirm = $state(null) // { svcName, envName, envValue }

  function requestMoveToEnvFile(svcName, envName, envValue) {
    envConfirm = { svcName, envName, envValue }
  }

  function confirmMoveToEnvFile() {
    if (!envConfirm) return
    const { svcName, envName, envValue } = envConfirm
    const existing = envFileVars.find((v) => v.key === envName)
    if (!existing) {
      envFileVars = [...envFileVars, { key: envName, value: envValue }]
    }
    updateServiceDirect(svcName, (s) => {
      if (!s.environment) s.environment = {}
      s.environment[envName] = `\${${envName}}`
    })
    saveEnvFile()
    envConfirm = null
  }

  function cancelMoveToEnvFile() {
    envConfirm = null
  }

  // Autocomplete state for $ references
  let envSuggestFor = $state(null) // { svcName, rowIdx }
  let envSuggestFilter = $state('')

  function getEnvSuggestions(filter) {
    const q = filter.replace(/^\$\{?/, '').toLowerCase()
    return envFileVars
      .filter(v => v.key && v.key.toLowerCase().includes(q))
      .slice(0, 6)
  }

  function applySuggestion(svcName, rowIdx, key, isPending) {
    const ref = `\${${key}}`
    if (isPending) {
      const p = pendingEnvRows[svcName] || []
      const pi = rowIdx - (parseEnv(compose.services?.[svcName]).length)
      const newPending = p.map((r, idx) => idx === pi ? { ...r, value: ref } : r)
      pendingEnvRows = { ...pendingEnvRows, [svcName]: newPending }
    } else {
      const envRows = parseEnv(compose.services?.[svcName])
      const newRows = envRows.map((r, idx) => idx === rowIdx ? { ...r, value: ref } : r)
      updateServiceDirect(svcName, (s) => {
        const serialized = serializeEnv(newRows)
        if (serialized) s.environment = serialized
        else delete s.environment
      })
    }
    envSuggestFor = null
    envSuggestFilter = ''
  }

  function moveFromEnvFile(svcName, envName) {
    // Find current value in .env
    const found = envFileVars.find((v) => v.key === envName)
    const val = found ? found.value : ''
    // Update compose env value to the actual value
    updateServiceDirect(svcName, (s) => {
      if (!s.environment) s.environment = {}
      s.environment[envName] = val
    })
  }

  // ---- Validation ----
  function validatePort(val, errKey) {
    if (val === '' || val == null) { clearError(errKey); return true }
    const n = Number(val)
    if (!Number.isInteger(n) || n < 1 || n > 65535) {
      setError(errKey, 'Must be 1-65535')
      return false
    }
    clearError(errKey)
    return true
  }

  function validatePct(val, errKey) {
    if (val === '' || val == null) { clearError(errKey); return true }
    const n = Number(val)
    if (isNaN(n) || n < 0 || n > 100) {
      setError(errKey, 'Must be 0-100')
      return false
    }
    clearError(errKey)
    return true
  }

  function validateNonNeg(val, errKey) {
    if (val === '' || val == null) { clearError(errKey); return true }
    const n = Number(val)
    if (isNaN(n) || n < 0) {
      setError(errKey, 'Must be >= 0')
      return false
    }
    clearError(errKey)
    return true
  }

  function validateMemory(val, errKey) {
    if (val === '' || val == null) { clearError(errKey); return true }
    const match = val.match(/^(\d+(?:\.\d+)?)\s*(b|k|kb|m|mb|g|gb|t|tb)?$/i)
    if (!match) {
      setError(errKey, 'Use a number with unit: b, k, m, g (e.g. 512m, 1g)')
      return false
    }
    const num = parseFloat(match[1])
    const unit = (match[2] || 'b').toLowerCase().replace(/b$/, '') || 'b'
    const multipliers = { b: 1, k: 1024, m: 1024 ** 2, g: 1024 ** 3, t: 1024 ** 4 }
    const bytes = num * (multipliers[unit] || 1)
    if (bytes < 6 * 1024 * 1024) {
      setError(errKey, 'Minimum 6MB (Docker requirement)')
      return false
    }
    clearError(errKey)
    return true
  }

  function validateCpu(val, errKey) {
    if (val === '' || val == null) { clearError(errKey); return true }
    const n = parseFloat(val)
    if (isNaN(n) || n <= 0 || !/^\d+(\.\d+)?$/.test(val)) {
      setError(errKey, 'Must be a positive number (e.g. 0.5, 2)')
      return false
    }
    clearError(errKey)
    return true
  }

  function validateImage(svcName, val) {
    const key = `services.${svcName}.image`
    if (!val || !val.trim()) {
      setError(key, 'Required')
      return false
    }
    clearError(key)
    return true
  }

  // ---- Input class helper ----
  function inputCls(errKey) {
    const base = 'w-full bg-input-bg border rounded px-2.5 py-1.5 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50'
    return errors[errKey] ? `${base} border-danger` : `${base} border-border`
  }
</script>

<div class="space-y-3">

  <!-- ======================== SECTION 1: Services ======================== -->
  <AccordionSection title="Services" expanded={true}>
    <div class="space-y-4">
      {#each serviceNames as svcName, svcIdx (svcName)}
        {@const svc = compose.services[svcName]}
        {@const depInfo = parseDependsOn(svc)}
        {@const otherServices = serviceNames.filter((s) => s !== svcName)}

        {@const envRows = parseEnv(svc)}
        {@const pending = pendingEnvRows[svcName] || []}
        {@const allEnvRows = [...envRows, ...pending]}
        <div class="bg-surface-1 border border-border rounded-lg p-4 space-y-3">
          <!-- Service header (click-to-edit name) -->
          {#if editingServiceName === svcName}
            <input
              type="text"
              bind:value={editingNameValue}
              onblur={() => commitRename(svcName)}
              onkeydown={(e) => { if (e.key === 'Enter') commitRename(svcName); if (e.key === 'Escape') editingServiceName = null }}
              class="text-sm font-semibold text-text-primary bg-input-bg border border-accent rounded px-2 py-0.5 focus:outline-none focus:ring-1 focus:ring-accent/50"
              autofocus
            />
          {:else}
            <button
              type="button"
              class="text-sm font-semibold text-text-primary hover:text-accent cursor-pointer flex items-center gap-1.5 group"
              onclick={() => startRename(svcName)}
              title="Click to rename"
            >
              {svcName}
              <svg class="w-3 h-3 text-text-muted opacity-0 group-hover:opacity-100 transition-opacity" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
              </svg>
            </button>
          {/if}

          <!-- Image -->
          <div>
            <label class="block text-xs text-text-secondary mb-1">
              Image <span class="text-danger">*</span>
            </label>
            <input
              type="text"
              value={svc.image ?? ''}
              placeholder="nginx:alpine"
              oninput={(e) => {
                validateImage(svcName, e.currentTarget.value)
                updateService(svcName, 'image', e.currentTarget.value)
              }}
              class={inputCls(`services.${svcName}.image`)}
            />
            {#if errors[`services.${svcName}.image`]}
              <p class="text-xs text-danger mt-0.5">{errors[`services.${svcName}.image`]}</p>
            {:else}
              <p class="text-xs text-text-muted mt-0.5">Docker image, e.g. postgres:16, nginx:alpine</p>
            {/if}
          </div>

          <!-- Environment Variables (enhanced with .env integration) -->
          <div class="space-y-2">
            <div>
              <span class="text-xs font-medium text-text-primary">Environment Variables</span>
              <span class="text-xs text-text-muted ml-1.5">Set env vars for this service</span>
            </div>
            {#each allEnvRows as row, i}
              {@const isPending = i >= envRows.length}
              {@const isRef = isEnvRef(row.value)}
              {@const resolved = isRef ? resolveEnvRef(row.value) : null}
              {@const showSuggestions = envSuggestFor?.svcName === svcName && envSuggestFor?.rowIdx === i}
              {@const suggestions = showSuggestions ? getEnvSuggestions(envSuggestFilter) : []}
              <div class="flex items-center gap-2">
                <input
                  type="text"
                  placeholder="KEY"
                  value={row.name}
                  oninput={(e) => {
                    const val = e.currentTarget.value
                    if (isPending) {
                      // Pending row got a name - move to compose state
                      const pi = i - envRows.length
                      if (val) {
                        const newPending = pending.filter((_, idx) => idx !== pi)
                        pendingEnvRows = { ...pendingEnvRows, [svcName]: newPending }
                        updateServiceDirect(svcName, (s) => {
                          if (!s.environment) s.environment = {}
                          s.environment[val] = row.value || ''
                        })
                      } else {
                        const newPending = pending.map((r, idx) => idx === pi ? { ...r, name: val } : r)
                        pendingEnvRows = { ...pendingEnvRows, [svcName]: newPending }
                      }
                    } else {
                      const newRows = envRows.map((r, idx) => idx === i ? { ...r, name: val } : r)
                      updateServiceDirect(svcName, (s) => {
                        const serialized = serializeEnv(newRows)
                        if (serialized) s.environment = serialized
                        else delete s.environment
                      })
                    }
                  }}
                  class="flex-1 bg-input-bg border border-border rounded px-2.5 py-1.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50 min-w-0"
                />
                <div class="flex-1 relative min-w-0">
                  <input
                    type="text"
                    placeholder="VALUE"
                    value={row.value}
                    oninput={(e) => {
                      const val = e.currentTarget.value
                      if (val.includes('$') && envFileVars.length > 0) {
                        envSuggestFor = { svcName, rowIdx: i }
                        envSuggestFilter = val
                      } else if (envSuggestFor?.svcName === svcName && envSuggestFor?.rowIdx === i) {
                        envSuggestFor = null
                      }
                      if (isPending) {
                        const pi = i - envRows.length
                        const newPending = pending.map((r, idx) => idx === pi ? { ...r, value: val } : r)
                        pendingEnvRows = { ...pendingEnvRows, [svcName]: newPending }
                      } else {
                        const newRows = envRows.map((r, idx) => idx === i ? { ...r, value: val } : r)
                        updateServiceDirect(svcName, (s) => {
                          const serialized = serializeEnv(newRows)
                          if (serialized) s.environment = serialized
                          else delete s.environment
                        })
                      }
                    }}
                    onblur={() => { setTimeout(() => { if (envSuggestFor?.rowIdx === i) envSuggestFor = null }, 150) }}
                    class="w-full bg-input-bg border border-border rounded px-2.5 py-1.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50 {isRef ? 'pr-16' : ''}"
                  />
                  {#if isRef}
                    <span class="absolute right-2 top-1/2 -translate-y-1/2 text-[10px] bg-accent/15 text-accent px-1.5 py-0.5 rounded font-medium" title={resolved != null ? `Resolved: ${resolved}` : 'Not found in .env'}>
                      .env
                    </span>
                  {/if}
                  {#if showSuggestions && suggestions.length > 0}
                    <div class="absolute left-0 top-full mt-1 w-full bg-surface-2 border border-border/50 rounded-lg shadow-xl z-20 py-1 max-h-40 overflow-y-auto">
                      {#each suggestions as s}
                        <button
                          type="button"
                          class="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-hover transition-colors flex items-center justify-between"
                          onmousedown={() => applySuggestion(svcName, i, s.key, isPending)}
                        >
                          <span class="font-mono text-text-primary">${'{' + s.key + '}'}</span>
                          <span class="text-text-muted truncate ml-2 max-w-32">{s.value}</span>
                        </button>
                      {/each}
                    </div>
                  {/if}
                </div>
                {#if slug && row.name && !isPending}
                  {#if isRef}
                    <button
                      type="button"
                      onclick={() => moveFromEnvFile(svcName, row.name)}
                      class="flex-shrink-0 p-1.5 rounded text-text-muted hover:text-accent hover:bg-accent/10 transition-colors"
                      title="Inline value (stop using .env)"
                    >
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M8 11V7a4 4 0 118 0m-4 8v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2z" />
                      </svg>
                    </button>
                  {:else}
                    <button
                      type="button"
                      onclick={() => requestMoveToEnvFile(svcName, row.name, row.value)}
                      class="flex-shrink-0 p-1.5 rounded text-text-muted hover:text-accent hover:bg-accent/10 transition-colors"
                      title="Move to .env file (shared across services)"
                    >
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                      </svg>
                    </button>
                  {/if}
                {/if}
                <button
                  type="button"
                  onclick={() => {
                    if (isPending) {
                      const pi = i - envRows.length
                      const newPending = pending.filter((_, idx) => idx !== pi)
                      pendingEnvRows = { ...pendingEnvRows, [svcName]: newPending }
                    } else {
                      const newRows = envRows.filter((_, idx) => idx !== i)
                      updateServiceDirect(svcName, (s) => {
                        const serialized = serializeEnv(newRows)
                        if (serialized) s.environment = serialized
                        else delete s.environment
                      })
                    }
                  }}
                  class="flex-shrink-0 p-1.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
                  aria-label="Remove row"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
              </div>
            {/each}
            <button
              type="button"
              onclick={() => {
                const newPending = [...pending, { name: '', value: '' }]
                pendingEnvRows = { ...pendingEnvRows, [svcName]: newPending }
              }}
              class="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary hover:bg-surface-3 px-2 py-1 rounded transition-colors"
            >
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
              </svg>
              Add
            </button>
          </div>

          <!-- Shared Variables (.env file) - only under first service -->
          {#if slug && svcIdx === 0}
            <div class="border border-border/50 rounded-lg overflow-hidden">
              <button
                type="button"
                class="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-surface-3/50 transition-colors bg-surface-2/50"
                onclick={() => envFileExpanded = !envFileExpanded}
              >
                <span class="text-xs font-medium text-text-primary">Shared Variables (.env file)</span>
                <svg
                  class="w-3.5 h-3.5 text-text-muted transition-transform duration-200 {envFileExpanded ? 'rotate-180' : ''}"
                  fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              {#if envFileExpanded}
                <div class="px-3 pb-3 pt-2 space-y-2">
                  <p class="text-xs text-text-muted">Shared across all services. Reference with <code class="font-mono text-accent/80">${'{VAR_NAME}'}</code>.</p>
                  {#if envFileLoading}
                    <p class="text-xs text-text-muted">Loading...</p>
                  {:else}
                    {#each envFileVars as v, i}
                      <div class="flex items-center gap-2">
                        <input
                          type="text"
                          value={v.key}
                          placeholder="KEY"
                          oninput={(e) => { envFileVars = envFileVars.map((ev, idx) => idx === i ? { ...ev, key: e.currentTarget.value } : ev) }}
                          class="flex-1 bg-input-bg border border-border rounded px-2.5 py-1.5 text-sm text-text-primary font-mono placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50 min-w-0"
                        />
                        <input
                          type="text"
                          value={v.value}
                          placeholder="value"
                          oninput={(e) => { envFileVars = envFileVars.map((ev, idx) => idx === i ? { ...ev, value: e.currentTarget.value } : ev) }}
                          class="flex-1 bg-input-bg border border-border rounded px-2.5 py-1.5 text-sm text-text-primary font-mono placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50 min-w-0"
                        />
                        <button
                          type="button"
                          onclick={() => { envFileVars = envFileVars.filter((_, idx) => idx !== i) }}
                          class="flex-shrink-0 p-1.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
                          aria-label="Remove"
                        >
                          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        </button>
                      </div>
                    {/each}
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        onclick={() => { envFileVars = [...envFileVars, { key: '', value: '' }] }}
                        class="flex items-center gap-1 text-xs text-text-muted hover:text-text-primary hover:bg-surface-3 px-2 py-1 rounded transition-colors"
                      >
                        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                          <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
                        </svg>
                        Add
                      </button>
                      <button
                        type="button"
                        onclick={saveEnvFile}
                        disabled={envFileSaving}
                        class="text-xs px-2.5 py-1 rounded bg-accent text-white hover:bg-accent/90 disabled:opacity-50 transition-colors"
                      >
                        {envFileSaving ? 'Saving...' : 'Save'}
                      </button>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}

          <!-- Volumes -->
          <RepeatableField
            label="Volumes"
            hint="Mount host path or named volume"
            rows={parseVolumes(svc)}
            fields={[
              { key: 'source', placeholder: 'Host path or volume' },
              { key: 'target', placeholder: 'Container path' },
            ]}
            onchange={(rows) => updateServiceDirect(svcName, (s) => {
              const serialized = serializeVolumes(rows)
              if (serialized.length) s.volumes = serialized
              else delete s.volumes
            })}
          />

          <!-- Restart Policy -->
          <div>
            <label class="block text-xs text-text-secondary mb-1">Restart Policy</label>
            <select
              value={svc.restart ?? 'unless-stopped'}
              onchange={(e) => updateService(svcName, 'restart', e.currentTarget.value)}
              class={inputCls(`services.${svcName}.restart`)}
            >
              <option value="no">no</option>
              <option value="always">always</option>
              <option value="unless-stopped">unless-stopped</option>
              <option value="on-failure">on-failure</option>
            </select>
            <p class="text-xs text-text-muted mt-0.5">When to restart the container</p>
          </div>

          <!-- Command -->
          <div>
            <label class="block text-xs text-text-secondary mb-1">Command</label>
            <input
              type="text"
              value={Array.isArray(svc.command) ? svc.command.join(' ') : (svc.command ?? '')}
              placeholder="node server.js"
              oninput={(e) => updateService(svcName, 'command', e.currentTarget.value)}
              class={inputCls(`services.${svcName}.command`)}
            />
            <p class="text-xs text-text-muted mt-0.5">Override the default container command</p>
          </div>

          <!-- Labels (non-SD) -->
          <RepeatableField
            label="Labels"
            hint="Custom Docker labels (simpledeploy.* labels are in Endpoint settings)"
            rows={parseLabels(svc)}
            fields={[
              { key: 'key', placeholder: 'Label key' },
              { key: 'value', placeholder: 'Label value' },
            ]}
            onchange={(rows) => updateServiceDirect(svcName, (s) => {
              const serialized = serializeLabels(rows, s.labels)
              if (serialized) s.labels = serialized
              else delete s.labels
            })}
          />

          <!-- Advanced (nested collapsible) -->
          <div class="pt-1">
            <AccordionSection title="Advanced" expanded={false}>
              <div class="space-y-4">

                <!-- Ports (moved to Advanced) -->
                <div>
                  <p class="text-xs text-blue-400 bg-blue-500/10 rounded-md px-3 py-2 mb-2">
                    For HTTP services, use Endpoint settings above. Port mappings expose directly on the host and bypass the reverse proxy.
                  </p>
                  <RepeatableField
                    label="Ports"
                    hint="Map host port to container port"
                    rows={parsePorts(svc)}
                    fields={[
                      { key: 'host', placeholder: 'Host port' },
                      { key: 'container', placeholder: 'Container port' },
                    ]}
                    onchange={(rows) => updateServiceDirect(svcName, (s) => {
                      const serialized = serializePorts(rows)
                      if (serialized.length) s.ports = serialized
                      else delete s.ports
                    })}
                  />
                </div>

                <!-- Resource Limits -->
                <div>
                  <p class="text-xs font-medium text-text-primary mb-2">Resource Limits</p>
                  <div class="grid grid-cols-2 gap-3">
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">CPU Limit</label>
                      <input
                        type="text"
                        value={svc.deploy?.resources?.limits?.cpus ?? ''}
                        placeholder="0.5"
                        oninput={(e) => {
                          if (validateCpu(e.currentTarget.value, `services.${svcName}.cpu_limit`)) updateService(svcName, 'deploy.resources.limits.cpus', e.currentTarget.value)
                        }}
                        class={inputCls(`services.${svcName}.cpu_limit`)}
                      />
                      {#if errors[`services.${svcName}.cpu_limit`]}
                        <p class="text-xs text-danger mt-0.5">{errors[`services.${svcName}.cpu_limit`]}</p>
                      {:else}
                        <p class="text-xs text-text-muted mt-0.5">Max CPUs, e.g. 0.5, 2</p>
                      {/if}
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">CPU Reservation</label>
                      <input
                        type="text"
                        value={svc.deploy?.resources?.reservations?.cpus ?? ''}
                        placeholder="0.25"
                        oninput={(e) => {
                          if (validateCpu(e.currentTarget.value, `services.${svcName}.cpu_res`)) updateService(svcName, 'deploy.resources.reservations.cpus', e.currentTarget.value)
                        }}
                        class={inputCls(`services.${svcName}.cpu_res`)}
                      />
                      {#if errors[`services.${svcName}.cpu_res`]}
                        <p class="text-xs text-danger mt-0.5">{errors[`services.${svcName}.cpu_res`]}</p>
                      {:else}
                        <p class="text-xs text-text-muted mt-0.5">Guaranteed CPUs, e.g. 0.25</p>
                      {/if}
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">Memory Limit</label>
                      <input
                        type="text"
                        value={svc.deploy?.resources?.limits?.memory ?? ''}
                        placeholder="512m"
                        oninput={(e) => {
                          if (validateMemory(e.currentTarget.value, `services.${svcName}.mem_limit`)) updateService(svcName, 'deploy.resources.limits.memory', e.currentTarget.value)
                        }}
                        class={inputCls(`services.${svcName}.mem_limit`)}
                      />
                      {#if errors[`services.${svcName}.mem_limit`]}
                        <p class="text-xs text-danger mt-0.5">{errors[`services.${svcName}.mem_limit`]}</p>
                      {:else}
                        <p class="text-xs text-text-muted mt-0.5">Min 6MB. Use b, k, m, g units (e.g. 512m, 1g)</p>
                      {/if}
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">Memory Reservation</label>
                      <input
                        type="text"
                        value={svc.deploy?.resources?.reservations?.memory ?? ''}
                        placeholder="256m"
                        oninput={(e) => {
                          if (validateMemory(e.currentTarget.value, `services.${svcName}.mem_res`)) updateService(svcName, 'deploy.resources.reservations.memory', e.currentTarget.value)
                        }}
                        class={inputCls(`services.${svcName}.mem_res`)}
                      />
                      {#if errors[`services.${svcName}.mem_res`]}
                        <p class="text-xs text-danger mt-0.5">{errors[`services.${svcName}.mem_res`]}</p>
                      {:else}
                        <p class="text-xs text-text-muted mt-0.5">Min 6MB. Use b, k, m, g units (e.g. 256m)</p>
                      {/if}
                    </div>
                  </div>
                </div>

                <!-- Health Check -->
                <div>
                  <p class="text-xs font-medium text-text-primary mb-2">Health Check</p>
                  <div class="grid grid-cols-2 gap-3">
                    <div class="col-span-2">
                      <label class="block text-xs text-text-secondary mb-1">Test Command</label>
                      <input
                        type="text"
                        value={Array.isArray(svc.healthcheck?.test)
                          ? svc.healthcheck.test.filter((t) => t !== 'CMD' && t !== 'CMD-SHELL').join(' ')
                          : (svc.healthcheck?.test ?? '')}
                        placeholder="curl -f http://localhost/"
                        oninput={(e) => updateService(svcName, 'healthcheck.test', e.currentTarget.value ? ['CMD-SHELL', e.currentTarget.value] : undefined)}
                        class={inputCls(`services.${svcName}.hc.test`)}
                      />
                      <p class="text-xs text-text-muted mt-0.5">Health check command, e.g. curl -f http://localhost/</p>
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">Interval</label>
                      <input
                        type="text"
                        value={svc.healthcheck?.interval ?? ''}
                        placeholder="30s"
                        oninput={(e) => updateService(svcName, 'healthcheck.interval', e.currentTarget.value)}
                        class={inputCls(`services.${svcName}.hc.interval`)}
                      />
                      <p class="text-xs text-text-muted mt-0.5">Time between checks, e.g. 30s</p>
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">Timeout</label>
                      <input
                        type="text"
                        value={svc.healthcheck?.timeout ?? ''}
                        placeholder="10s"
                        oninput={(e) => updateService(svcName, 'healthcheck.timeout', e.currentTarget.value)}
                        class={inputCls(`services.${svcName}.hc.timeout`)}
                      />
                      <p class="text-xs text-text-muted mt-0.5">Max time for check, e.g. 10s</p>
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">Retries</label>
                      <input
                        type="number"
                        value={svc.healthcheck?.retries ?? ''}
                        placeholder="3"
                        oninput={(e) => updateService(svcName, 'healthcheck.retries', e.currentTarget.value ? Number(e.currentTarget.value) : undefined)}
                        class={inputCls(`services.${svcName}.hc.retries`)}
                      />
                      <p class="text-xs text-text-muted mt-0.5">Failures before unhealthy</p>
                    </div>
                    <div>
                      <label class="block text-xs text-text-secondary mb-1">Start Period</label>
                      <input
                        type="text"
                        value={svc.healthcheck?.start_period ?? ''}
                        placeholder="30s"
                        oninput={(e) => updateService(svcName, 'healthcheck.start_period', e.currentTarget.value)}
                        class={inputCls(`services.${svcName}.hc.start_period`)}
                      />
                      <p class="text-xs text-text-muted mt-0.5">Grace period for startup, e.g. 30s</p>
                    </div>
                  </div>
                </div>

                <!-- Dependencies (per-dependency conditions) -->
                {#if otherServices.length > 0}
                  <div>
                    <p class="text-xs font-medium text-text-primary mb-1">Dependencies</p>
                    <p class="text-xs text-text-muted mb-2">Services that must start before this one</p>
                    <div class="space-y-1.5 mb-2">
                      {#each otherServices as dep (dep)}
                        {@const isChecked = depInfo.services.includes(dep)}
                        <div class="flex items-center gap-2">
                          <label class="flex items-center gap-2 cursor-pointer flex-1">
                            <input
                              type="checkbox"
                              checked={isChecked}
                              onchange={(e) => {
                                const checked = e.currentTarget.checked
                                const current = depInfo.services
                                const next = checked
                                  ? [...current, dep]
                                  : current.filter((s) => s !== dep)
                                const conditions = { ...depInfo.conditions }
                                if (checked) conditions[dep] = 'service_started'
                                else delete conditions[dep]
                                updateService(svcName, 'depends_on', serializeDependsOn(next, conditions))
                              }}
                              class="rounded border-border bg-input-bg accent-accent"
                            />
                            <span class="text-sm text-text-primary">{dep}</span>
                          </label>
                          {#if isChecked}
                            <select
                              value={depInfo.conditions[dep] || 'service_started'}
                              onchange={(e) => {
                                const conditions = { ...depInfo.conditions, [dep]: e.currentTarget.value }
                                updateService(svcName, 'depends_on', serializeDependsOn(depInfo.services, conditions))
                              }}
                              class="text-xs bg-input-bg border border-border rounded px-2 py-1 text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50"
                            >
                              <option value="service_started">service_started</option>
                              <option value="service_healthy">service_healthy</option>
                            </select>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  </div>
                {/if}

              </div>
            </AccordionSection>
          </div>
        </div>
      {/each}

      <!-- Add Service -->
      <button
        type="button"
        onclick={addService}
        class="flex items-center gap-1.5 text-sm text-text-muted hover:text-text-primary hover:bg-surface-3 px-3 py-2 rounded-md border border-dashed border-border transition-colors w-full justify-center"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
        </svg>
        Add Service
      </button>
    </div>
  </AccordionSection>

  <!-- ======================== SECTION 3: Rate Limiting ======================== -->
  <AccordionSection title="Rate Limiting" expanded={false}>
    {#if !firstService}
      <p class="text-xs text-text-muted">Add a service to configure rate limiting.</p>
    {:else}
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">

        <!-- Requests per window -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Requests per window</label>
          <input
            type="number"
            value={getLabel(firstService, 'ratelimit.requests')}
            placeholder="100"
            oninput={(e) => {
              if (validateNonNeg(e.currentTarget.value, 'sd.rl.requests')) setLabel('ratelimit.requests', e.currentTarget.value)
            }}
            class={inputCls('sd.rl.requests')}
          />
          {#if errors['sd.rl.requests']}
            <p class="text-xs text-danger mt-0.5">{errors['sd.rl.requests']}</p>
          {:else}
            <p class="text-xs text-text-muted mt-0.5">Max requests per window</p>
          {/if}
        </div>

        <!-- Window -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Window</label>
          <input
            type="text"
            value={getLabel(firstService, 'ratelimit.window')}
            placeholder="1m"
            oninput={(e) => setLabel('ratelimit.window', e.currentTarget.value)}
            class={inputCls('sd.rl.window')}
          />
          <p class="text-xs text-text-muted mt-0.5">Time window, e.g. 1m, 5m, 1h</p>
        </div>

        <!-- Burst -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Burst</label>
          <input
            type="number"
            value={getLabel(firstService, 'ratelimit.burst')}
            placeholder="20"
            oninput={(e) => {
              if (validateNonNeg(e.currentTarget.value, 'sd.rl.burst')) setLabel('ratelimit.burst', e.currentTarget.value)
            }}
            class={inputCls('sd.rl.burst')}
          />
          {#if errors['sd.rl.burst']}
            <p class="text-xs text-danger mt-0.5">{errors['sd.rl.burst']}</p>
          {:else}
            <p class="text-xs text-text-muted mt-0.5">Extra burst allowance above limit</p>
          {/if}
        </div>

        <!-- Limit by -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Limit by</label>
          <select
            value={getLabel(firstService, 'ratelimit.by') || 'ip'}
            onchange={(e) => setLabel('ratelimit.by', e.currentTarget.value)}
            class={inputCls('sd.rl.by')}
          >
            <option value="ip">ip</option>
            <option value="header">header</option>
          </select>
          <p class="text-xs text-text-muted mt-0.5">What to rate limit by</p>
        </div>

      </div>
    {/if}
  </AccordionSection>

  <!-- ======================== SECTION 4: Networks & Volumes ======================== -->
  <AccordionSection title="Networks & Volumes" expanded={false}>
    <div class="space-y-5">

      <!-- Named Volumes -->
      <RepeatableField
        label="Named Volumes"
        hint="Top-level named volume definitions"
        rows={parseTopVolumes()}
        fields={[
          { key: 'name', placeholder: 'Volume name' },
          { key: 'driver', placeholder: 'Driver' },
        ]}
        onchange={(rows) => {
          const updated = deepClone(compose)
          const serialized = serializeTopVolumes(rows)
          if (serialized) updated.volumes = serialized
          else delete updated.volumes
          emitChange(updated)
        }}
      />
      {#if serviceNames.length > 0 && Object.keys(compose.volumes || {}).length > 0}
        <div>
          <p class="text-xs font-medium text-text-primary mb-1">Volume references</p>
          <div class="space-y-1">
            {#each Object.keys(compose.volumes || {}) as volName}
              {@const refs = serviceNames.filter((s) => {
                const svols = compose.services[s]?.volumes || []
                return svols.some((v) => {
                  const src = typeof v === 'string' ? v.split(':')[0] : v.source
                  return src === volName
                })
              })}
              <div class="text-xs text-text-secondary bg-surface-1 rounded px-2 py-1">
                <span class="font-medium text-text-primary">{volName}:</span>
                {refs.length ? refs.join(', ') : 'not referenced'}
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Divider -->
      <div class="border-t border-border/50"></div>

      <!-- Networks -->
      <RepeatableField
        label="Networks"
        hint="Top-level network definitions"
        rows={parseNetworks()}
        fields={[
          { key: 'name', placeholder: 'Network name' },
          { key: 'driver', placeholder: 'Driver (bridge)' },
        ]}
        onchange={(rows) => {
          const updated = deepClone(compose)
          const serialized = serializeNetworks(rows)
          if (serialized) updated.networks = serialized
          else delete updated.networks
          emitChange(updated)
        }}
      />
      {#if serviceNames.length > 0}
        {@const svcWithNetworks = serviceNames.filter((s) => compose.services[s]?.networks)}
        {#if svcWithNetworks.length > 0}
          <div>
            <p class="text-xs font-medium text-text-primary mb-1">Service network assignments</p>
            <div class="space-y-1">
              {#each svcWithNetworks as svcName}
                {@const svcNetworks = compose.services[svcName]?.networks}
                <div class="text-xs text-text-secondary bg-surface-1 rounded px-2 py-1">
                  <span class="font-medium text-text-primary">{svcName}:</span>
                  {Array.isArray(svcNetworks) ? svcNetworks.join(', ') : Object.keys(svcNetworks).join(', ')}
                </div>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </AccordionSection>

</div>

<!-- Confirmation dialog for moving env var to .env file -->
{#if envConfirm}
  <div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
    <button class="absolute inset-0 bg-black/50 backdrop-blur-sm" onclick={cancelMoveToEnvFile} aria-label="Close"></button>
    <div class="relative bg-surface-2 border border-border/50 rounded-2xl p-6 min-w-80 max-w-md shadow-2xl animate-scale-in">
      <h3 class="text-lg font-semibold text-text-primary tracking-tight mb-2">Move to .env file?</h3>
      <p class="text-sm text-text-secondary mb-1">
        This will store <span class="font-mono font-medium text-text-primary">{envConfirm.envName}</span> in the shared <code class="font-mono text-[11px]">.env</code> file and replace the value in compose with <span class="font-mono text-accent">${'{' + envConfirm.envName + '}'}</span>.
      </p>
      <p class="text-xs text-text-muted mb-5">The .env file is shared across all services and loaded automatically by Docker Compose.</p>
      <div class="flex justify-end gap-2">
        <button onclick={cancelMoveToEnvFile} class="px-4 py-2 text-sm border border-border/50 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors">Cancel</button>
        <button onclick={confirmMoveToEnvFile} class="px-4 py-2 text-sm bg-btn-primary text-surface-0 rounded-lg hover:bg-btn-primary-hover transition-colors shadow-sm">Move to .env</button>
      </div>
    </div>
  </div>
{/if}
