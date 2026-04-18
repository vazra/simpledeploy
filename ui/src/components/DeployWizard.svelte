<script>
  import { onDestroy } from 'svelte'
  import yaml from 'js-yaml'
  import Button from './Button.svelte'
  import YamlEditor from './YamlEditor.svelte'
  import VisualEditor from './VisualEditor.svelte'
  import TemplatePicker from './TemplatePicker.svelte'
  import { appTemplates, categories, applyVars, suggestName } from '../lib/appTemplates.js'
  import { api } from '../lib/api.js'

  let { open = false, onclose = () => {}, onComplete = () => {}, initialTemplateId = null } = $props()

  let step = $state(0)
  const steps = ['Start', 'Configure', 'Review', 'Deploy']

  // Step 0 view: 'chooser' shows the three start options, 'templates' drops into the picker.
  let startView = $state('chooser')

  let existingAppNames = $state([])

  // Fetch existing app names when wizard opens (for suggestName dedup).
  $effect(() => {
    if (open && existingAppNames.length === 0) {
      api.listApps().then((res) => {
        if (!res.error && Array.isArray(res.data)) {
          existingAppNames = res.data.map((a) => a.name).filter(Boolean)
        }
      }).catch(() => { /* fail silently */ })
    }
  })

  function handleTemplateApply({ template, vars }) {
    const resolved = applyVars(template.compose, vars)
    compose = resolved
    composeText = yaml.dump(resolved, { lineWidth: -1 })
    appName = suggestName(template.nameSuggestion, existingAppNames)
    nameError = ''
    editorMode = 'visual'
    scheduleValidation(composeText)
    step = 1
  }

  function handleBlank() {
    compose = { services: { web: { image: '' } } }
    composeText = ''
    editorMode = 'visual'
    step = 1
  }

  function handleStartUpload() {
    document.getElementById('wizard-start-upload')?.click()
  }

  function handleStartUploadFile(e) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const text = reader.result
      composeText = text
      try {
        compose = yaml.load(text) || { services: {} }
      } catch { compose = { services: {} } }
      editorMode = 'yaml'
      scheduleValidation(text)
      step = 1
    }
    reader.readAsText(file)
    e.target.value = ''
  }

  // Step 1 state
  let appName = $state('')
  let nameError = $state('')
  let editorMode = $state('visual') // 'visual' | 'yaml'

  // Compose state (shared between modes)
  let compose = $state({ services: { web: { image: '' } } })
  let composeText = $state('')
  let validating = $state(false)
  let composeValid = $state(false)
  let composeErrors = $state([])
  let validateTimer = $state(null)
  let visualErrors = $state({})

  const NAME_REGEX = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$/

  function validateName(val) {
    if (!val.trim()) return 'App name is required'
    if (!NAME_REGEX.test(val)) return 'Must start with alphanumeric, then alphanumeric/dot/hyphen/underscore, max 63 chars'
    return ''
  }

  function handleNameInput(e) {
    appName = e.currentTarget.value
    nameError = appName.trim() ? validateName(appName) : ''
  }

  function encodeBase64(str) {
    return btoa(String.fromCodePoint(...new TextEncoder().encode(str)))
  }

  // Mode switching (same pattern as ConfigTab)
  function switchMode(newMode) {
    if (newMode === editorMode) return
    if (newMode === 'yaml') {
      try {
        composeText = yaml.dump(compose, { lineWidth: -1 })
        composeValid = false
        composeErrors = []
        // Auto-validate the generated YAML
        scheduleValidation(composeText)
      } catch (e) {
        composeErrors = [e.message]
      }
    } else {
      try {
        compose = yaml.load(composeText) || { services: {} }
        composeErrors = []
      } catch (e) {
        composeErrors = ['Cannot switch to Visual: YAML has syntax errors']
        return
      }
    }
    editorMode = newMode
  }

  // Visual mode handlers
  function handleVisualChange(updated) {
    compose = updated
    composeValid = false
    composeErrors = []
    // Validate by converting to YAML
    try {
      const text = yaml.dump(compose, { lineWidth: -1 })
      scheduleValidation(text)
    } catch { /* ignore */ }
  }

  function handleVisualErrors(errs) {
    visualErrors = errs
  }

  // YAML mode handlers
  function handleYamlChange(val) {
    composeText = val
    composeValid = false
    composeErrors = []
    scheduleValidation(val)
  }

  function scheduleValidation(text) {
    if (validateTimer) clearTimeout(validateTimer)
    if (text.trim()) {
      validateTimer = setTimeout(() => runValidation(text), 800)
    }
  }

  async function runValidation(text) {
    validating = true
    try {
      const encoded = encodeBase64(text)
      const res = await api.validateCompose(encoded)
      if (res.data?.valid) {
        composeValid = true
        composeErrors = []
      } else {
        composeValid = false
        composeErrors = res.data?.errors || ['Invalid compose file']
      }
    } catch {
      composeValid = false
      composeErrors = ['Validation failed (network error)']
    }
    validating = false
  }

  function handleFileUpload(e) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const text = reader.result
      composeText = text
      try {
        compose = yaml.load(text) || { services: {} }
      } catch { /* stay in yaml mode */ }
      handleYamlChange(text)
    }
    reader.readAsText(file)
  }

  // Check if compose has at least one service with an image
  let hasServices = $derived.by(() => {
    const svcs = compose?.services || {}
    return Object.values(svcs).some(s => s.image?.trim())
  })

  let canProceed = $derived(
    appName.trim() && !nameError && composeValid && !Object.keys(visualErrors).length
  )

  // Warn if unsubstituted template placeholders remain.
  let hasStrayTokens = $derived(/\{\{\s*\w+\s*\}\}/.test(composeText))

  // Step 2: build review summary from compose object
  let reviewServices = $derived.by(() => {
    if (!compose?.services) return []
    return Object.entries(compose.services).map(([name, svc]) => ({
      name,
      image: svc.image || '',
      ports: (svc.ports || []).map(p => typeof p === 'string' ? p : `${p.published}:${p.target}`),
      envCount: Array.isArray(svc.environment) ? svc.environment.length : Object.keys(svc.environment || {}).length,
      volumeCount: (svc.volumes || []).length,
    }))
  })

  let reviewLabels = $derived.by(() => {
    if (!compose?.services) return {}
    const labels = {}
    for (const svc of Object.values(compose.services)) {
      for (const [k, v] of Object.entries(svc.labels || {})) {
        if (k.startsWith('simpledeploy.')) labels[k] = v
      }
    }
    return labels
  })

  function enterStep2() {
    // Sync compose text from visual mode before review
    if (editorMode === 'visual') {
      try {
        composeText = yaml.dump(compose, { lineWidth: -1 })
      } catch { /* already validated */ }
    } else {
      try {
        compose = yaml.load(composeText) || { services: {} }
      } catch { /* already validated */ }
    }
    step = 2
  }

  // Step 3 state
  let deployStatus = $state('deploying')
  let deployLines = $state([])
  let currentAction = $state('')
  let deployWs = $state(null)
  let logContainer = $state(null)
  let confirmOverwrite = $state(false)

  async function startDeploy(force = false) {
    step = 3
    deployStatus = 'deploying'
    deployLines = []
    currentAction = 'Starting deploy...'

    // Always deploy from YAML text
    if (editorMode === 'visual') {
      try { composeText = yaml.dump(compose, { lineWidth: -1 }) } catch { /* validated */ }
    }

    const encoded = encodeBase64(composeText)
    const res = await api.deploy(appName.trim(), encoded, force)

    if (res.status === 409) {
      // App already exists, ask for confirmation
      deployStatus = 'deploying'
      step = 2
      confirmOverwrite = true
      return
    }

    if (res.error) {
      deployStatus = 'failed'
      deployLines = [{ line: res.error, stream: 'stderr' }]
      return
    }

    deployWs = api.deployLogsWs(appName.trim())
    deployWs.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      if (msg.done) {
        deployStatus = msg.action === 'deploy' ? 'success' : 'failed'
        currentAction = msg.action === 'deploy' ? 'Deploy complete' : `Failed: ${msg.action}`
        deployWs?.close()
        deployWs = null
        return
      }
      if (msg.line) {
        deployLines = [...deployLines, msg]
        if (logContainer) {
          requestAnimationFrame(() => { logContainer.scrollTop = logContainer.scrollHeight })
        }
      }
      if (msg.action) currentAction = msg.action
    }
    deployWs.onerror = () => {
      deployStatus = 'failed'
      currentAction = 'WebSocket connection failed'
    }
    deployWs.onclose = () => {
      if (deployStatus === 'deploying') {
        deployStatus = 'failed'
        currentAction = 'Connection lost'
      }
    }
  }

  onDestroy(() => {
    if (deployWs) deployWs.close()
    if (validateTimer) clearTimeout(validateTimer)
  })

  function resetWizard() {
    step = 0
    startView = 'chooser'
    appName = ''
    editorMode = 'visual'
    compose = { services: { web: { image: '' } } }
    composeText = ''
    composeValid = false
    composeErrors = []
    visualErrors = {}
    nameError = ''
    deployStatus = 'deploying'
    deployLines = []
    currentAction = ''
    existingAppNames = []
  }

  let confirmClose = $state(false)

  function handleClose() {
    if (step === 3 && deployStatus === 'deploying') {
      confirmClose = true
      return
    }
    resetWizard()
    onclose()
  }

  function onKeydown(e) {
    if (open && e.key === 'Escape') handleClose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
<div class="fixed inset-0 z-40" role="dialog" aria-modal="true">
  <button class="absolute inset-0 bg-black/40 backdrop-blur-sm" onclick={handleClose} aria-label="Close"></button>
  <div class="absolute inset-4 sm:inset-6 lg:inset-10 bg-surface-2 border border-border/50 rounded-2xl shadow-2xl flex flex-col animate-scale-in overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b border-border/50 shrink-0">
      <div class="flex items-center gap-4">
        <h3 class="text-lg font-semibold text-text-primary tracking-tight">Deploy App</h3>
        <!-- Step indicator -->
        <div class="flex items-center gap-1.5">
          {#each steps as label, i}
            {@const active = step === i}
            {@const done = step > i}
            {#if i > 0}
              <div class="w-6 h-px {done ? 'bg-accent' : 'bg-border/50'}"></div>
            {/if}
            <div class="flex items-center gap-1">
              <div class="w-5 h-5 rounded-full flex items-center justify-center text-[10px] font-medium
                {active ? 'bg-accent text-white' : done ? 'bg-accent/20 text-accent' : 'bg-surface-3 text-text-muted'}">
                {#if done}
                  <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
                {:else}
                  {i + 1}
                {/if}
              </div>
              <span class="text-xs {active ? 'text-text-primary font-medium' : 'text-text-muted'}">{label}</span>
            </div>
          {/each}
        </div>
      </div>
      <button onclick={handleClose} class="text-text-secondary hover:text-text-primary p-1" aria-label="Close">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto px-6 py-5">
      {#if step === 0}
        {#if startView === 'chooser' && !initialTemplateId}
          <div class="max-w-3xl mx-auto flex flex-col gap-6">
            <div class="text-center">
              <h4 class="text-base font-semibold text-text-primary">How would you like to start?</h4>
              <p class="text-xs text-text-muted mt-1">Choose an option below. You can switch between visual and YAML anytime.</p>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <button
                type="button"
                onclick={handleStartUpload}
                class="text-left bg-surface-3/50 border border-border/30 rounded-lg px-5 py-4 hover:border-accent/50 transition-colors flex flex-col gap-2"
              >
                <div class="flex items-center gap-2">
                  <svg class="w-5 h-5 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M7 10l5-5m0 0l5 5m-5-5v12" />
                  </svg>
                  <span class="text-sm font-semibold text-text-primary">Upload docker-compose file</span>
                </div>
                <p class="text-xs text-text-muted">I already have a <span class="font-mono">docker-compose.yml</span>. Upload and deploy it.</p>
              </button>

              <button
                type="button"
                onclick={handleBlank}
                class="text-left bg-surface-3/50 border border-border/30 rounded-lg px-5 py-4 hover:border-accent/50 transition-colors flex flex-col gap-2"
              >
                <div class="flex items-center gap-2">
                  <svg class="w-5 h-5 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
                  </svg>
                  <span class="text-sm font-semibold text-text-primary">Build it yourself</span>
                </div>
                <p class="text-xs text-text-muted">Use the visual builder to add services, ports, env vars, and volumes step by step.</p>
              </button>
            </div>

            <input
              id="wizard-start-upload"
              type="file"
              accept=".yml,.yaml"
              onchange={handleStartUploadFile}
              class="hidden"
            />

            <div class="border-t border-border/30 pt-4 text-center">
              <p class="text-xs text-text-muted mb-2">Not sure where to begin? Browse ready-made templates to learn example configs or quickly try something out.</p>
              <button
                type="button"
                onclick={() => startView = 'templates'}
                class="inline-flex items-center gap-1 text-sm font-medium text-accent hover:underline"
              >
                Browse templates
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" /></svg>
              </button>
            </div>
          </div>
        {:else}
          {#if !initialTemplateId}
            <button
              type="button"
              onclick={() => startView = 'chooser'}
              class="inline-flex items-center gap-1 text-xs text-text-muted hover:text-text-primary mb-3"
            >
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" /></svg>
              Back
            </button>
          {/if}
          <TemplatePicker
            templates={appTemplates}
            {categories}
            {initialTemplateId}
            onapply={handleTemplateApply}
            onblank={handleBlank}
          />
        {/if}
      {:else if step === 1}
        <div class="max-w-3xl mx-auto flex flex-col gap-5">
          <!-- App Name -->
          <div>
            <label class="block text-sm font-medium text-text-primary mb-1.5">App Name</label>
            <input
              value={appName}
              oninput={handleNameInput}
              placeholder="my-app"
              class="w-full max-w-sm px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
                {nameError ? 'border-danger/50' : 'border-border/50'}"
            />
            {#if nameError}
              <p class="text-xs text-danger mt-1">{nameError}</p>
            {:else}
              <p class="text-xs text-text-muted mt-1">Alphanumeric, dots, hyphens, underscores. Max 63 chars.</p>
            {/if}
          </div>

          <!-- Editor mode toggle -->
          <div>
            <div class="flex items-center justify-between mb-3">
              <label class="block text-sm font-medium text-text-primary">Compose Configuration</label>
              <div class="flex items-center gap-1 bg-surface-3/40 rounded-lg p-0.5">
                <button
                  type="button"
                  onclick={() => switchMode('visual')}
                  class="px-3 py-1 text-xs font-medium rounded-md transition-colors
                    {editorMode === 'visual' ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
                >Visual</button>
                <button
                  type="button"
                  onclick={() => switchMode('yaml')}
                  class="px-3 py-1 text-xs font-medium rounded-md transition-colors
                    {editorMode === 'yaml' ? 'bg-surface-2 text-text-primary shadow-sm' : 'text-text-muted hover:text-text-primary'}"
                >YAML</button>
              </div>
            </div>

            {#if hasStrayTokens}
              <div class="flex items-start gap-1.5 text-xs text-warning mb-2 bg-warning/10 border border-warning/20 rounded-md px-2.5 py-1.5">
                <svg class="w-3.5 h-3.5 mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" /></svg>
                <span>Template placeholders still present — values were not substituted. Fix before deploying.</span>
              </div>
            {/if}

            <!-- Validation status -->
            {#if validating}
              <div class="flex items-center gap-2 text-xs text-text-muted mb-2">
                <svg class="animate-spin h-3 w-3" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path></svg>
                Validating...
              </div>
            {:else if composeValid}
              <div class="flex items-center gap-1.5 text-xs text-success mb-2">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
                Valid compose file
              </div>
            {:else if composeErrors.length > 0}
              <div class="text-xs text-danger mb-2 flex flex-col gap-1">
                {#each composeErrors as err}
                  <p>{err}</p>
                {/each}
              </div>
            {/if}

            {#if editorMode === 'visual'}
              <VisualEditor {compose} onchange={handleVisualChange} onerrors={handleVisualErrors} />
            {:else}
              <div class="flex items-center gap-2 mb-2">
                <button
                  type="button"
                  onclick={() => document.getElementById('deploy-file-upload')?.click()}
                  class="px-2.5 py-1 text-xs rounded border border-border/50 text-text-muted hover:text-text-primary transition-colors"
                >Upload file</button>
                <input
                  id="deploy-file-upload"
                  type="file"
                  accept=".yml,.yaml"
                  onchange={handleFileUpload}
                  class="hidden"
                />
              </div>
              <YamlEditor value={composeText} onchange={handleYamlChange} />
            {/if}
          </div>
        </div>

      {:else if step === 2}
        <div class="max-w-3xl mx-auto flex flex-col gap-5">
          <div>
            <h4 class="text-sm font-medium text-text-primary mb-1">App: {appName}</h4>
            <p class="text-xs text-text-muted">Review your configuration before deploying.</p>
          </div>

          <!-- Services -->
          <div>
            <h4 class="text-xs font-medium text-text-muted mb-2 uppercase tracking-wider">Services</h4>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {#each reviewServices as svc}
                <div class="bg-surface-3/50 border border-border/30 rounded-lg px-4 py-3">
                  <div class="flex items-center gap-2 mb-1">
                    <span class="text-sm font-medium text-text-primary">{svc.name}</span>
                    {#each svc.ports as port}
                      <span class="text-xs bg-accent/10 text-accent px-1.5 py-0.5 rounded">{port}</span>
                    {/each}
                  </div>
                  {#if svc.image}
                    <p class="text-xs text-text-muted font-mono">{svc.image}</p>
                  {/if}
                  <div class="flex gap-3 mt-1.5 text-xs text-text-muted">
                    {#if svc.envCount > 0}
                      <span>{svc.envCount} env var{svc.envCount > 1 ? 's' : ''}</span>
                    {/if}
                    {#if svc.volumeCount > 0}
                      <span>{svc.volumeCount} volume{svc.volumeCount > 1 ? 's' : ''}</span>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          </div>

          <!-- SimpleDeploy labels if any -->
          {#if Object.keys(reviewLabels).length > 0}
            <div>
              <h4 class="text-xs font-medium text-text-muted mb-2 uppercase tracking-wider">Routing & Settings</h4>
              <div class="bg-surface-3/50 border border-border/30 rounded-lg px-4 py-3">
                <div class="grid grid-cols-2 gap-2">
                  {#each Object.entries(reviewLabels) as [key, val]}
                    <div>
                      <span class="text-xs text-text-muted">{key.replace('simpledeploy.', '')}</span>
                      <p class="text-sm text-text-primary font-mono">{val}</p>
                    </div>
                  {/each}
                </div>
              </div>
            </div>
          {/if}
        </div>

      {:else}
        <div class="max-w-3xl mx-auto flex flex-col gap-4">
          <!-- Status badge -->
          <div class="flex items-center gap-2">
            {#if deployStatus === 'deploying'}
              <span class="flex items-center gap-1.5 text-sm font-medium text-warning">
                <span class="w-2 h-2 rounded-full bg-warning animate-pulse"></span>
                Deploying...
              </span>
            {:else if deployStatus === 'success'}
              <span class="flex items-center gap-1.5 text-sm font-medium text-success">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
                Deployed
              </span>
            {:else}
              <span class="flex items-center gap-1.5 text-sm font-medium text-danger">
                <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
                Failed
              </span>
            {/if}
            {#if currentAction}
              <span class="text-xs text-text-muted">{currentAction}</span>
            {/if}
          </div>

          <!-- Log viewer -->
          <div
            bind:this={logContainer}
            class="min-h-[300px] max-h-[500px] overflow-y-auto bg-[#0c0c0c] light:bg-[#1a1a2e] rounded-lg font-mono text-[13px] leading-5 p-4"
          >
            {#if deployLines.length === 0}
              <div class="flex items-center justify-center h-full min-h-[280px] text-[#555] text-sm">Waiting for output...</div>
            {:else}
              {#each deployLines as line}
                <div class="whitespace-pre-wrap break-all py-px {/\b(error|fatal|fail)/i.test(line.line) ? 'text-red-400' : 'text-[#d4d4d4] light:text-[#c8c8d8]'}">
                  {line.line}
                </div>
              {/each}
            {/if}
          </div>

          <!-- Actions -->
          <div class="flex gap-2">
            {#if deployStatus === 'success'}
              <Button size="sm" onclick={() => { onComplete(); window.location.hash = `#/apps/${appName.trim()}` }}>View App</Button>
              <Button size="sm" variant="secondary" onclick={resetWizard}>Deploy Another</Button>
            {:else if deployStatus === 'failed'}
              <Button size="sm" variant="secondary" onclick={() => { step = 1 }}>Back to Edit</Button>
              <Button size="sm" variant="secondary" onclick={resetWizard}>Start Over</Button>
            {:else}
              <Button size="sm" variant="secondary" onclick={() => { deployWs?.close(); step = 1 }}>Back to Edit</Button>
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <!-- Footer -->
    {#if step === 1 || step === 2}
      <div class="flex justify-between px-6 py-4 border-t border-border/50 shrink-0">
        {#if step === 2}
          <Button variant="secondary" size="sm" onclick={() => { step = 1; confirmOverwrite = false }}>Back</Button>
          <Button size="sm" onclick={() => startDeploy(false)}>Deploy</Button>
        {:else}
          <Button variant="secondary" size="sm" onclick={() => step = 0}>Back</Button>
          <Button size="sm" disabled={!canProceed} onclick={enterStep2}>Next</Button>
        {/if}
      </div>
    {/if}
  </div>
</div>
{/if}

<!-- Overwrite confirmation -->
{#if confirmOverwrite}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <button class="absolute inset-0 bg-black/40" onclick={() => confirmOverwrite = false} aria-label="Cancel"></button>
    <div class="relative bg-surface-2 border border-border/50 rounded-xl p-6 max-w-sm shadow-2xl">
      <p class="text-sm text-text-primary mb-1 font-medium">App already exists</p>
      <p class="text-xs text-text-muted mb-4">"{appName}" is already deployed. This will update the existing app with your new configuration.</p>
      <div class="flex gap-2 justify-end">
        <Button size="sm" variant="secondary" onclick={() => confirmOverwrite = false}>Cancel</Button>
        <Button size="sm" variant="danger" onclick={() => { confirmOverwrite = false; startDeploy(true) }}>Redeploy</Button>
      </div>
    </div>
  </div>
{/if}

<!-- Close confirmation during deploy -->
{#if confirmClose}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <button class="absolute inset-0 bg-black/40" onclick={() => confirmClose = false} aria-label="Cancel"></button>
    <div class="relative bg-surface-2 border border-border/50 rounded-xl p-6 max-w-sm shadow-2xl">
      <p class="text-sm text-text-primary mb-4">Deploy in progress. Close anyway?</p>
      <div class="flex gap-2 justify-end">
        <Button size="sm" variant="secondary" onclick={() => confirmClose = false}>Cancel</Button>
        <Button size="sm" variant="danger" onclick={() => { deployWs?.close(); resetWizard(); onclose() }}>Close</Button>
      </div>
    </div>
  </div>
{/if}
