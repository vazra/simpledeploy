<script>
  import { onDestroy } from 'svelte'
  import Button from './Button.svelte'
  import YamlEditor from './YamlEditor.svelte'
  import AccordionSection from './AccordionSection.svelte'
  import SlidePanel from './SlidePanel.svelte'
  import { api } from '../lib/api.js'

  let { open = false, onclose = () => {}, onComplete = () => {} } = $props()

  let step = $state(1)

  const steps = ['Compose', 'Review', 'Deploy']

  // Step 1 state
  let appName = $state('')
  let nameError = $state('')

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

  // Compose state
  let composeText = $state('')
  let composeInputMode = $state('paste')
  let validating = $state(false)
  let composeValid = $state(false)
  let composeErrors = $state([])
  let validateTimer = $state(null)

  function handleComposeChange(val) {
    composeText = val
    composeValid = false
    composeErrors = []
    if (validateTimer) clearTimeout(validateTimer)
    if (val.trim()) {
      validateTimer = setTimeout(() => validateCompose(val), 800)
    }
  }

  async function validateCompose(text) {
    validating = true
    try {
      const encoded = btoa(unescape(encodeURIComponent(text)))
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
      composeText = reader.result
      handleComposeChange(composeText)
    }
    reader.readAsText(file)
  }

  // Step 2 state
  let parsedServices = $state([])

  // Routing labels
  let routingDomain = $state('')
  let routingPort = $state('')
  let routingTls = $state(false)

  function injectLabels(yaml) {
    if (!routingDomain && !routingPort && !routingTls) return yaml

    const labels = []
    if (routingDomain) labels.push(`      simpledeploy.domain: "${routingDomain}"`)
    if (routingPort) labels.push(`      simpledeploy.port: "${routingPort}"`)
    if (routingTls) labels.push(`      simpledeploy.tls: "letsencrypt"`)

    const labelBlock = labels.join('\n')
    const lines = yaml.split('\n')
    let inServices = false
    let firstServiceIndent = -1

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]
      const stripped = line.trimEnd()
      if (/^services:\s*$/.test(stripped)) { inServices = true; continue }
      if (inServices && stripped.trim() && !stripped.startsWith('#')) {
        const indent = line.search(/\S/)
        if (firstServiceIndent === -1) firstServiceIndent = indent

        if (indent === firstServiceIndent) {
          for (let j = i + 1; j < lines.length; j++) {
            const sline = lines[j]
            const sindent = sline.search(/\S/)
            if (sindent <= firstServiceIndent && sline.trim()) break
            if (sline.trim() === 'labels:') {
              lines.splice(j + 1, 0, labelBlock)
              return lines.join('\n')
            }
          }
          const labelsHeader = ' '.repeat(firstServiceIndent + 2) + 'labels:'
          lines.splice(i + 1, 0, labelsHeader, labelBlock)
          return lines.join('\n')
        }
      }
    }
    return yaml
  }

  // Step 3 state
  let deployStatus = $state('deploying') // 'deploying' | 'success' | 'failed'
  let deployLines = $state([])
  let currentAction = $state('')
  let deployWs = $state(null)
  let logContainer = $state(null)

  async function startDeploy() {
    step = 3
    deployStatus = 'deploying'
    deployLines = []
    currentAction = 'Starting deploy...'

    const finalCompose = injectLabels(composeText)
    const encoded = btoa(unescape(encodeURIComponent(finalCompose)))
    const res = await api.deploy(appName.trim(), encoded)

    if (res.error) {
      deployStatus = 'failed'
      deployLines = [{ line: res.error, stream: 'stderr' }]
      return
    }

    // Connect to deploy log WebSocket
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
    step = 1
    appName = ''
    composeText = ''
    composeValid = false
    composeErrors = []
    nameError = ''
    deployStatus = 'deploying'
    deployLines = []
    currentAction = ''
    parsedServices = []
    routingDomain = ''
    routingPort = ''
    routingTls = false
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

  function parseServicesFromYaml(text) {
    const services = []
    const lines = text.split('\n')
    let inServices = false
    let currentService = null
    let serviceIndent = -1

    for (const line of lines) {
      const stripped = line.trimEnd()
      if (stripped === '' || stripped.startsWith('#')) continue
      const indent = line.search(/\S/)

      if (/^services:\s*$/.test(stripped)) {
        inServices = true
        serviceIndent = -1
        continue
      }

      if (inServices) {
        if (indent === 0) { inServices = false; continue }

        if (serviceIndent === -1) serviceIndent = indent
        if (indent === serviceIndent && stripped.endsWith(':')) {
          currentService = { name: stripped.replace(':', '').trim(), image: '', ports: [] }
          services.push(currentService)
          continue
        }

        if (currentService) {
          const trimmed = stripped.trim()
          if (trimmed.startsWith('image:')) {
            currentService.image = trimmed.replace('image:', '').trim().replace(/['"]/g, '')
          }
          if (trimmed === 'ports:') currentService._inPorts = true
          else if (currentService._inPorts && trimmed.startsWith('- ')) {
            const port = trimmed.replace('- ', '').replace(/['"]/g, '').trim()
            if (/^\d/.test(port)) currentService.ports.push(port)
          } else if (!trimmed.startsWith('- ')) {
            currentService._inPorts = false
          }
        }
      }
    }
    return services.map(({ _inPorts, ...s }) => s)
  }

  async function enterStep2() {
    parsedServices = parseServicesFromYaml(composeText)
    step = 2
  }
</script>

<SlidePanel title="Deploy App" {open} onclose={handleClose}>
<div class="flex flex-col h-full">
  <!-- Step indicator -->
  <div class="flex items-center gap-2 mb-6">
    {#each steps as label, i}
      {@const num = i + 1}
      {@const active = step === num}
      {@const done = step > num}
      <div class="flex items-center gap-2 {i > 0 ? 'flex-1' : ''}">
        {#if i > 0}
          <div class="flex-1 h-px {done ? 'bg-accent' : 'bg-border/50'}"></div>
        {/if}
        <div class="flex items-center gap-1.5">
          <div class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium transition-colors
            {active ? 'bg-accent text-white' : done ? 'bg-accent/20 text-accent' : 'bg-surface-3 text-text-muted'}">
            {#if done}
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
            {:else}
              {num}
            {/if}
          </div>
          <span class="text-xs font-medium {active ? 'text-text-primary' : 'text-text-muted'}">{label}</span>
        </div>
      </div>
    {/each}
  </div>

  <!-- Step content -->
  <div class="flex-1 overflow-y-auto">
    {#if step === 1}
      <div class="flex flex-col gap-4">
        <div>
          <label class="block text-xs font-medium text-text-muted mb-2">App Name</label>
          <input
            value={appName}
            oninput={handleNameInput}
            placeholder="my-app"
            class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
              {nameError ? 'border-danger/50' : 'border-border/50'}"
          />
          {#if nameError}
            <p class="text-xs text-danger mt-1">{nameError}</p>
          {:else}
            <p class="text-xs text-text-muted mt-1">Alphanumeric, dots, hyphens, underscores. Max 63 chars.</p>
          {/if}
        </div>

        <div>
          <label class="block text-xs font-medium text-text-muted mb-2">Compose File</label>
          <div class="flex gap-1 mb-2">
            <button
              type="button"
              onclick={() => composeInputMode = 'paste'}
              class="px-2 py-1 text-xs rounded border transition-colors {composeInputMode === 'paste' ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
            >Paste</button>
            <button
              type="button"
              onclick={() => composeInputMode = 'upload'}
              class="px-2 py-1 text-xs rounded border transition-colors {composeInputMode === 'upload' ? 'bg-accent/10 border-accent/30 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
            >Upload</button>
          </div>

          {#if composeInputMode === 'paste'}
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
            <YamlEditor value={composeText} onchange={handleComposeChange} />
          {:else}
            <input
              type="file"
              accept=".yml,.yaml"
              onchange={handleFileUpload}
              class="w-full text-sm text-text-secondary file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border file:border-border file:text-sm file:bg-surface-3 file:text-text-primary hover:file:bg-surface-3/80"
            />
            {#if composeText}
              <p class="text-xs text-success mt-1">File loaded ({composeText.length} chars)</p>
            {/if}
          {/if}
        </div>
      </div>
    {:else if step === 2}
      <div class="flex flex-col gap-4">
        <!-- Service summary -->
        <div>
          <h4 class="text-xs font-medium text-text-muted mb-2">Services</h4>
          <div class="flex flex-col gap-2">
            {#each parsedServices as svc}
              <div class="bg-surface-3/50 border border-border/30 rounded-lg px-3 py-2.5">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-text-primary">{svc.name}</span>
                  {#if svc.ports.length > 0}
                    {#each svc.ports as port}
                      <span class="text-xs bg-accent/10 text-accent px-1.5 py-0.5 rounded">{port}</span>
                    {/each}
                  {/if}
                </div>
                {#if svc.image}
                  <p class="text-xs text-text-muted mt-1 font-mono">{svc.image}</p>
                {/if}
              </div>
            {/each}
            {#if parsedServices.length === 0}
              <p class="text-xs text-text-muted">Could not parse services (compose is still valid)</p>
            {/if}
          </div>
        </div>

        <!-- Quick routing labels -->
        <AccordionSection title="Configure Routing (optional)">
          <div class="flex flex-col gap-3">
            <div>
              <label class="block text-xs font-medium text-text-muted mb-1">Domain</label>
              <input
                bind:value={routingDomain}
                placeholder="app.example.com"
                class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/30"
              />
            </div>
            <div>
              <label class="block text-xs font-medium text-text-muted mb-1">Port</label>
              <input
                bind:value={routingPort}
                placeholder="8080"
                class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/30"
              />
            </div>
            <label class="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" bind:checked={routingTls} class="rounded border-border accent-accent" />
              <span class="text-xs text-text-primary">Enable TLS (Let's Encrypt)</span>
            </label>
          </div>
        </AccordionSection>
      </div>
    {:else}
      <div class="flex flex-col gap-4 h-full">
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
          class="flex-1 min-h-[300px] max-h-[400px] overflow-y-auto bg-[#0c0c0c] light:bg-[#1a1a2e] rounded-lg font-mono text-[13px] leading-5 p-4"
        >
          {#if deployLines.length === 0}
            <div class="flex items-center justify-center h-full text-[#555] text-sm">Waiting for output...</div>
          {:else}
            {#each deployLines as line}
              <div class="whitespace-pre-wrap break-all py-px {line.stream === 'stderr' ? 'text-red-400' : 'text-[#d4d4d4] light:text-[#c8c8d8]'}">
                {line.line}
              </div>
            {/each}
          {/if}
        </div>

        <!-- Completion actions -->
        {#if deployStatus === 'success'}
          <div class="flex gap-2">
            <Button size="sm" onclick={() => { onComplete(); window.location.hash = `#/apps/${appName.trim()}` }}>View App</Button>
            <Button size="sm" variant="secondary" onclick={resetWizard}>Deploy Another</Button>
          </div>
        {:else if deployStatus === 'failed'}
          <div class="flex gap-2">
            <Button size="sm" variant="secondary" onclick={() => { step = 1 }}>Back to Edit</Button>
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Footer -->
  {#if step < 3}
    <div class="flex justify-between pt-4 border-t border-border/50 mt-4">
      {#if step === 2}
        <Button variant="secondary" size="sm" onclick={() => step = 1}>Back</Button>
        <Button size="sm" onclick={startDeploy}>Deploy</Button>
      {:else}
        <div></div>
        <Button size="sm" disabled={!appName.trim() || !!nameError || !composeValid} onclick={enterStep2}>Next</Button>
      {/if}
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
          <Button size="sm" variant="danger" onclick={() => { deployWs?.close(); onclose() }}>Close</Button>
        </div>
      </div>
    </div>
  {/if}
</div>
</SlidePanel>
