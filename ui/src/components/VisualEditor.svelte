<script>
  import AccordionSection from './AccordionSection.svelte'
  import RepeatableField from './RepeatableField.svelte'

  let { compose = {}, onchange = () => {}, onerrors = () => {} } = $props()

  // ---- Errors ----
  let errors = $state({})

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

  // ---- SECTION 1: SimpleDeploy labels ----
  const SD_PREFIX = 'simpledeploy.'

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

  // ---- depends_on parsing ----
  function parseDependsOn(svc) {
    const raw = svc?.depends_on
    if (!raw) return { services: [], condition: 'service_started' }
    if (Array.isArray(raw)) return { services: raw, condition: 'service_started' }
    const services = Object.keys(raw)
    const condition = services.length ? (raw[services[0]]?.condition ?? 'service_started') : 'service_started'
    return { services, condition }
  }

  function serializeDependsOn(svcs, condition) {
    if (!svcs.length) return undefined
    if (condition === 'service_started') return svcs
    const obj = {}
    for (const s of svcs) obj[s] = { condition }
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

  <!-- ======================== SECTION 1: SimpleDeploy Settings ======================== -->
  <AccordionSection title="SimpleDeploy Settings" expanded={true}>
    {#if !firstService}
      <p class="text-xs text-text-muted">Add a service to configure SimpleDeploy settings.</p>
    {:else}
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">

        <!-- Domain -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Domain</label>
          <input
            type="text"
            value={getLabel(firstService, 'domain')}
            placeholder="myapp.example.com"
            oninput={(e) => setLabel('domain', e.currentTarget.value)}
            class={inputCls('sd.domain')}
          />
          <p class="text-xs text-text-muted mt-0.5">Public domain for this app, e.g. myapp.example.com</p>
        </div>

        <!-- Port -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Port</label>
          <input
            type="number"
            value={getLabel(firstService, 'port')}
            placeholder="3000"
            oninput={(e) => {
              if (validatePort(e.currentTarget.value, 'sd.port')) setLabel('port', e.currentTarget.value)
            }}
            class={inputCls('sd.port')}
          />
          {#if errors['sd.port']}
            <p class="text-xs text-danger mt-0.5">{errors['sd.port']}</p>
          {:else}
            <p class="text-xs text-text-muted mt-0.5">Container port to expose, e.g. 3000, 8080</p>
          {/if}
        </div>

        <!-- TLS -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">TLS</label>
          <select
            value={getLabel(firstService, 'tls') || 'letsencrypt'}
            onchange={(e) => setLabel('tls', e.currentTarget.value)}
            class={inputCls('sd.tls')}
          >
            <option value="letsencrypt">letsencrypt</option>
            <option value="custom">custom</option>
            <option value="off">off</option>
          </select>
          <p class="text-xs text-text-muted mt-0.5">How to handle HTTPS</p>
        </div>

        <!-- Rate Limit Requests -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Rate Limit - Requests</label>
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

        <!-- Rate Limit Window -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Rate Limit - Window</label>
          <input
            type="text"
            value={getLabel(firstService, 'ratelimit.window')}
            placeholder="1m"
            oninput={(e) => setLabel('ratelimit.window', e.currentTarget.value)}
            class={inputCls('sd.rl.window')}
          />
          <p class="text-xs text-text-muted mt-0.5">Time window, e.g. 1m, 5m, 1h</p>
        </div>

        <!-- Rate Limit Burst -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Rate Limit - Burst</label>
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

        <!-- Rate Limit By -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Rate Limit - By</label>
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

        <!-- Alert CPU -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Alert CPU %</label>
          <input
            type="number"
            value={getLabel(firstService, 'alert.cpu')}
            placeholder="80"
            oninput={(e) => {
              if (validatePct(e.currentTarget.value, 'sd.alert.cpu')) setLabel('alert.cpu', e.currentTarget.value)
            }}
            class={inputCls('sd.alert.cpu')}
          />
          {#if errors['sd.alert.cpu']}
            <p class="text-xs text-danger mt-0.5">{errors['sd.alert.cpu']}</p>
          {:else}
            <p class="text-xs text-text-muted mt-0.5">Alert when CPU exceeds this %</p>
          {/if}
        </div>

        <!-- Alert Memory -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Alert Memory %</label>
          <input
            type="number"
            value={getLabel(firstService, 'alert.memory')}
            placeholder="80"
            oninput={(e) => {
              if (validatePct(e.currentTarget.value, 'sd.alert.mem')) setLabel('alert.memory', e.currentTarget.value)
            }}
            class={inputCls('sd.alert.mem')}
          />
          {#if errors['sd.alert.mem']}
            <p class="text-xs text-danger mt-0.5">{errors['sd.alert.mem']}</p>
          {:else}
            <p class="text-xs text-text-muted mt-0.5">Alert when memory exceeds this %</p>
          {/if}
        </div>

        <!-- Path Patterns -->
        <div>
          <label class="block text-xs text-text-secondary mb-1">Path Patterns</label>
          <input
            type="text"
            value={getLabel(firstService, 'paths')}
            placeholder="/api,/health"
            oninput={(e) => setLabel('paths', e.currentTarget.value)}
            class={inputCls('sd.paths')}
          />
          <p class="text-xs text-text-muted mt-0.5">URL paths to track, comma-separated</p>
        </div>

      </div>
    {/if}
  </AccordionSection>

  <!-- ======================== SECTION 2: Services ======================== -->
  <AccordionSection title="Services" expanded={true}>
    <div class="space-y-4">
      {#each serviceNames as svcName (svcName)}
        {@const svc = compose.services[svcName]}
        {@const depInfo = parseDependsOn(svc)}
        {@const otherServices = serviceNames.filter((s) => s !== svcName)}

        <div class="bg-surface-1 border border-border rounded-lg p-4 space-y-3">
          <!-- Service header -->
          <h4 class="text-sm font-semibold text-text-primary">{svcName}</h4>

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

          <!-- Ports -->
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

          <!-- Environment Variables -->
          <RepeatableField
            label="Environment Variables"
            hint="Set env vars for this service"
            rows={parseEnv(svc)}
            fields={[
              { key: 'name', placeholder: 'KEY' },
              { key: 'value', placeholder: 'VALUE' },
            ]}
            onchange={(rows) => updateServiceDirect(svcName, (s) => {
              const serialized = serializeEnv(rows)
              if (serialized) s.environment = serialized
              else delete s.environment
            })}
          />

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
            hint="Custom Docker labels (simpledeploy.* labels are in SimpleDeploy Settings)"
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

                <!-- Dependencies -->
                {#if otherServices.length > 0}
                  <div>
                    <p class="text-xs font-medium text-text-primary mb-1">Dependencies</p>
                    <p class="text-xs text-text-muted mb-2">Services that must start before this one</p>
                    <div class="space-y-1 mb-2">
                      {#each otherServices as dep (dep)}
                        <label class="flex items-center gap-2 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={depInfo.services.includes(dep)}
                            onchange={(e) => {
                              const checked = e.currentTarget.checked
                              const current = depInfo.services
                              const next = checked
                                ? [...current, dep]
                                : current.filter((s) => s !== dep)
                              updateService(svcName, 'depends_on', serializeDependsOn(next, depInfo.condition))
                            }}
                            class="rounded border-border bg-input-bg accent-accent"
                          />
                          <span class="text-sm text-text-primary">{dep}</span>
                        </label>
                      {/each}
                    </div>
                    {#if depInfo.services.length > 0}
                      <div>
                        <label class="block text-xs text-text-secondary mb-1">Condition</label>
                        <select
                          value={depInfo.condition}
                          onchange={(e) => {
                            updateService(svcName, 'depends_on', serializeDependsOn(depInfo.services, e.currentTarget.value))
                          }}
                          class={inputCls(`services.${svcName}.dep.condition`)}
                        >
                          <option value="service_started">service_started</option>
                          <option value="service_healthy">service_healthy</option>
                        </select>
                        <p class="text-xs text-text-muted mt-0.5">When to consider dependency ready</p>
                      </div>
                    {/if}
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

  <!-- ======================== SECTION 3: Networking ======================== -->
  <AccordionSection title="Networking" expanded={false}>
    <div class="space-y-3">
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
        <div>
          <p class="text-xs font-medium text-text-primary mb-1">Service network assignments</p>
          <div class="space-y-1">
            {#each serviceNames as svcName}
              {@const svcNetworks = compose.services[svcName]?.networks}
              {#if svcNetworks}
                <div class="text-xs text-text-secondary bg-surface-1 rounded px-2 py-1">
                  <span class="font-medium text-text-primary">{svcName}:</span>
                  {Array.isArray(svcNetworks) ? svcNetworks.join(', ') : Object.keys(svcNetworks).join(', ')}
                </div>
              {/if}
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </AccordionSection>

  <!-- ======================== SECTION 4: Volumes ======================== -->
  <AccordionSection title="Volumes" expanded={false}>
    <div class="space-y-3">
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
      {#if serviceNames.length > 0}
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
    </div>
  </AccordionSection>

</div>
