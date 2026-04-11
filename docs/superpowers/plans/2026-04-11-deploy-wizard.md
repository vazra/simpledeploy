# Deploy Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-form deploy slide panel with a 3-step wizard: compose input with validation, review/configure, and deploy with live log streaming.

**Architecture:** New `DeployWizard.svelte` component owns all wizard state and renders inside the existing `SlidePanel`. Dashboard just toggles `showDeployPanel` and passes `onComplete` callback. No backend changes needed.

**Tech Stack:** Svelte 5 (runes), existing components (SlidePanel, YamlEditor, Button, AccordionSection, Badge), existing API (`deploy`, `validateCompose`, `listRegistries`, `deployLogsWs`).

**Spec:** `docs/superpowers/specs/2026-04-11-deploy-wizard-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `ui/src/components/DeployWizard.svelte` | Create | Full wizard: 3-step state machine, validation, label injection, deploy log streaming |
| `ui/src/routes/Dashboard.svelte` | Modify | Remove inline deploy form state/logic, render `<DeployWizard>` inside SlidePanel |

---

### Task 1: Create DeployWizard skeleton with step indicator

**Files:**
- Create: `ui/src/components/DeployWizard.svelte`

- [ ] **Step 1: Create the component with step state and indicator UI**

```svelte
<script>
  import Button from './Button.svelte'

  let { onclose = () => {}, onComplete = () => {} } = $props()

  let step = $state(1)

  const steps = ['Compose', 'Review', 'Deploy']
</script>

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
      <p class="text-text-muted text-sm">Step 1 placeholder</p>
    {:else if step === 2}
      <p class="text-text-muted text-sm">Step 2 placeholder</p>
    {:else}
      <p class="text-text-muted text-sm">Step 3 placeholder</p>
    {/if}
  </div>

  <!-- Footer -->
  <div class="flex justify-between pt-4 border-t border-border/50 mt-4">
    {#if step > 1 && step < 3}
      <Button variant="secondary" size="sm" onclick={() => step--}>Back</Button>
    {:else}
      <div></div>
    {/if}
    {#if step < 3}
      <Button size="sm" onclick={() => step++}>Next</Button>
    {/if}
  </div>
</div>
```

- [ ] **Step 2: Verify it renders**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds (component not wired in yet, just checking for syntax errors in the build)

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/DeployWizard.svelte
git commit -m "feat(ui): scaffold DeployWizard with step indicator"
```

---

### Task 2: Wire DeployWizard into Dashboard, remove old deploy form

**Files:**
- Modify: `ui/src/routes/Dashboard.svelte`

- [ ] **Step 1: Replace inline deploy state/logic with DeployWizard import**

In `Dashboard.svelte`, remove these lines (35-61):

```
  // Deploy form
  let showDeployPanel = $state(false)
  let deployName = $state('')
  let deployCompose = $state('')
  let deployInputMode = $state('paste')
  let deploying = $state(false)

  async function handleDeploy() { ... }
  function handleFileUpload(e) { ... }
```

Replace with:

```javascript
  import DeployWizard from '../components/DeployWizard.svelte'

  let showDeployPanel = $state(false)
```

Add the import at the top with the other imports. Keep `showDeployPanel`.

- [ ] **Step 2: Replace the SlidePanel contents**

Replace lines 435-488 (the entire `<SlidePanel>...</SlidePanel>` block) with:

```svelte
  <SlidePanel title="Deploy App" open={showDeployPanel} onclose={() => showDeployPanel = false}>
    <DeployWizard onclose={() => showDeployPanel = false} onComplete={() => { showDeployPanel = false; loadDashboard() }} />
  </SlidePanel>
```

- [ ] **Step 3: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds. The deploy panel now shows the wizard skeleton.

- [ ] **Step 4: Commit**

```bash
git add ui/src/routes/Dashboard.svelte
git commit -m "feat(ui): wire DeployWizard into Dashboard, remove old deploy form"
```

---

### Task 3: Implement Step 1 - Name input with validation

**Files:**
- Modify: `ui/src/components/DeployWizard.svelte`

- [ ] **Step 1: Add name state and validation**

Add to the `<script>` block after the existing state:

```javascript
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
```

- [ ] **Step 2: Replace step 1 placeholder with name input UI**

Replace `<p class="text-text-muted text-sm">Step 1 placeholder</p>` with:

```svelte
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

        <!-- Compose editor placeholder for next task -->
        <p class="text-text-muted text-sm">Compose editor coming next</p>
      </div>
```

- [ ] **Step 3: Disable Next when name is invalid**

Update the Next button in the footer:

```svelte
    {#if step === 1}
      <Button size="sm" disabled={!appName.trim() || !!nameError} onclick={() => step++}>Next</Button>
    {:else if step < 3}
      <Button size="sm" onclick={() => step++}>Next</Button>
    {/if}
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/DeployWizard.svelte
git commit -m "feat(ui): deploy wizard step 1 name input with validation"
```

---

### Task 4: Implement Step 1 - YamlEditor with compose validation

**Files:**
- Modify: `ui/src/components/DeployWizard.svelte`

- [ ] **Step 1: Add compose state and validation logic**

Add import at top:

```javascript
  import YamlEditor from './YamlEditor.svelte'
  import { api } from '../lib/api.js'
```

Add state after the name validation code:

```javascript
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
    const encoded = btoa(text)
    const res = await api.validateCompose(encoded)
    validating = false
    if (res.data?.valid) {
      composeValid = true
      composeErrors = []
    } else {
      composeValid = false
      composeErrors = res.data?.errors || ['Invalid compose file']
    }
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
```

- [ ] **Step 2: Replace compose placeholder with YamlEditor UI**

Replace `<!-- Compose editor placeholder for next task -->` and the `<p>` after it with:

```svelte
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
```

- [ ] **Step 3: Update Next button to require valid compose**

Update the step 1 Next button disabled condition:

```svelte
      <Button size="sm" disabled={!appName.trim() || !!nameError || !composeValid} onclick={() => step++}>Next</Button>
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/DeployWizard.svelte
git commit -m "feat(ui): deploy wizard step 1 YamlEditor with auto-validation"
```

---

### Task 5: Implement Step 2 - Service summary and registry selector

**Files:**
- Modify: `ui/src/components/DeployWizard.svelte`

- [ ] **Step 1: Add client-side YAML parsing and registry state**

Add after the compose validation code:

```javascript
  // Step 2 state
  let parsedServices = $state([])
  let registries = $state([])
  let selectedRegistry = $state('')

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
        // Top-level key other than a service (indent 0) ends services block
        if (indent === 0) { inServices = false; continue }

        // Service name line (first indent level under services)
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
          if (trimmed.startsWith('- ') && lines[lines.indexOf(line) - 1]?.trim() === 'ports:' ||
              (currentService._inPorts && trimmed.startsWith('- '))) {
            // Simple port detection
            if (trimmed.startsWith('- ')) {
              const port = trimmed.replace('- ', '').replace(/['"]/g, '').trim()
              if (/^\d/.test(port)) currentService.ports.push(port)
            }
          }
          if (trimmed === 'ports:') currentService._inPorts = true
          else if (!trimmed.startsWith('- ')) currentService._inPorts = false
        }
      }
    }
    return services.map(({ _inPorts, ...s }) => s)
  }

  function isPrivateImage(image) {
    const parts = image.split('/')
    return parts.length > 1 && parts[0].includes('.')
  }

  async function enterStep2() {
    parsedServices = parseServicesFromYaml(composeText)
    const hasPrivate = parsedServices.some(s => isPrivateImage(s.image))
    if (hasPrivate) {
      const res = await api.listRegistries()
      registries = res.data || []
    }
    step = 2
  }
```

- [ ] **Step 2: Update Next button on step 1 to call enterStep2**

Change the step 1 Next button:

```svelte
      <Button size="sm" disabled={!appName.trim() || !!nameError || !composeValid} onclick={enterStep2}>Next</Button>
```

- [ ] **Step 3: Replace step 2 placeholder with service summary and registry UI**

Replace `<p class="text-text-muted text-sm">Step 2 placeholder</p>` with:

```svelte
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

        <!-- Registry selector -->
        {#if registries.length > 0}
          <div>
            <label class="block text-xs font-medium text-text-muted mb-2">Private Registry</label>
            <select
              bind:value={selectedRegistry}
              class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/30"
            >
              <option value="">None (public images)</option>
              {#each registries as reg}
                <option value={reg.id}>{reg.name} ({reg.url})</option>
              {/each}
            </select>
          </div>
        {/if}
      </div>
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/DeployWizard.svelte
git commit -m "feat(ui): deploy wizard step 2 service summary and registry selector"
```

---

### Task 6: Implement Step 2 - Quick routing labels

**Files:**
- Modify: `ui/src/components/DeployWizard.svelte`

- [ ] **Step 1: Add routing label state**

Add import at top:

```javascript
  import AccordionSection from './AccordionSection.svelte'
```

Add state after the registry code:

```javascript
  // Routing labels
  let routingDomain = $state('')
  let routingPort = $state('')
  let routingTls = $state(false)
```

- [ ] **Step 2: Add routing accordion after registry selector in step 2**

Insert before the closing `</div>` of step 2's flex container (after the registry selector block):

```svelte
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
```

- [ ] **Step 3: Add label injection function**

Add after the routing state:

```javascript
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
    let insertIdx = -1

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]
      const stripped = line.trimEnd()
      if (/^services:\s*$/.test(stripped)) { inServices = true; continue }
      if (inServices && stripped.trim() && !stripped.startsWith('#')) {
        const indent = line.search(/\S/)
        if (firstServiceIndent === -1) firstServiceIndent = indent

        if (indent === firstServiceIndent) {
          // Found first service. Look for existing labels: key
          for (let j = i + 1; j < lines.length; j++) {
            const sline = lines[j]
            const sindent = sline.search(/\S/)
            if (sindent <= firstServiceIndent && sline.trim()) break // next service or top-level key
            if (sline.trim() === 'labels:') {
              // Insert after labels: line
              lines.splice(j + 1, 0, labelBlock)
              return lines.join('\n')
            }
          }
          // No labels key found, insert one after the service name line
          const labelsHeader = ' '.repeat(firstServiceIndent + 2) + 'labels:'
          lines.splice(i + 1, 0, labelsHeader, labelBlock)
          return lines.join('\n')
        }
      }
    }
    return yaml
  }
```

- [ ] **Step 4: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/DeployWizard.svelte
git commit -m "feat(ui): deploy wizard step 2 routing labels with injection"
```

---

### Task 7: Implement Step 3 - Deploy with live log streaming

**Files:**
- Modify: `ui/src/components/DeployWizard.svelte`

- [ ] **Step 1: Add deploy state and functions**

Add after the `injectLabels` function:

```javascript
  import { onDestroy } from 'svelte'

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
    const encoded = btoa(finalCompose)
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
    registries = []
    selectedRegistry = ''
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
    onclose()
  }
```

- [ ] **Step 2: Replace step 3 placeholder with deploy UI**

Replace `<p class="text-text-muted text-sm">Step 3 placeholder</p>` with:

```svelte
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
```

- [ ] **Step 3: Update footer to use startDeploy and hide nav on step 3**

Update the footer section:

```svelte
  <!-- Footer -->
  <div class="flex justify-between pt-4 border-t border-border/50 mt-4">
    {#if step === 2}
      <Button variant="secondary" size="sm" onclick={() => step = 1}>Back</Button>
      <Button size="sm" onclick={startDeploy}>Deploy</Button>
    {:else if step === 1}
      <div></div>
      <Button size="sm" disabled={!appName.trim() || !!nameError || !composeValid} onclick={enterStep2}>Next</Button>
    {:else}
      <!-- Step 3: no footer nav, actions are inline -->
    {/if}
  </div>
```

- [ ] **Step 4: Add close confirmation modal**

Add at the bottom of the component template, before the closing `</div>`:

```svelte
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
```

- [ ] **Step 5: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add ui/src/components/DeployWizard.svelte
git commit -m "feat(ui): deploy wizard step 3 with live log streaming"
```

---

### Task 8: Integration polish and testing

**Files:**
- Modify: `ui/src/components/DeployWizard.svelte`
- Modify: `ui/src/routes/Dashboard.svelte`

- [ ] **Step 1: Pass handleClose to SlidePanel**

In `Dashboard.svelte`, the SlidePanel `onclose` should call the wizard's close handler. Update the SlidePanel:

The current approach has a problem: `SlidePanel`'s `onclose` fires when user clicks backdrop/Escape, but the wizard's `handleClose` (with deploy-in-progress guard) is internal. Fix by passing `onclose` as a prop through.

In `DeployWizard.svelte`, expose the handleClose by making the wizard itself not use SlidePanel. Instead, the Dashboard wraps it. The `onclose` prop already points to `() => showDeployPanel = false`. We need the wizard to intercept this.

Update `Dashboard.svelte` SlidePanel to let the wizard control closing:

```svelte
  <SlidePanel title="Deploy App" open={showDeployPanel} onclose={() => showDeployPanel = false}>
    <DeployWizard
      onclose={() => showDeployPanel = false}
      onComplete={() => { showDeployPanel = false; loadDashboard() }}
    />
  </SlidePanel>
```

This is already correct. The wizard's `handleClose` is only needed internally for the confirmation. But the SlidePanel's backdrop click bypasses it. To fix, make Dashboard's onclose go through the wizard.

Add a `let wizardRef = $state(null)` isn't needed. Simpler: export `isDeploying` from the wizard as a derived value, and use it in Dashboard.

In `DeployWizard.svelte`, add a prop the parent can check. Actually, the simplest fix: In the wizard, handle the parent's close via the `onclose` prop itself. The wizard already uses `handleClose` internally. The issue is that SlidePanel calls its own `onclose` directly.

Simplest approach: move SlidePanel inside DeployWizard so the wizard controls everything.

Update `DeployWizard.svelte` to wrap its content in SlidePanel:

Add to props:

```javascript
  let { open = false, onclose = () => {}, onComplete = () => {} } = $props()
```

Wrap the entire template in:

```svelte
<SlidePanel title="Deploy App" {open} onclose={handleClose}>
  ... existing wizard content ...
</SlidePanel>
```

Add import:

```javascript
  import SlidePanel from './SlidePanel.svelte'
```

Update `Dashboard.svelte` to remove the wrapping SlidePanel:

```svelte
  <DeployWizard open={showDeployPanel} onclose={() => showDeployPanel = false} onComplete={() => { showDeployPanel = false; loadDashboard() }} />
```

Remove the `SlidePanel` import from Dashboard if it's no longer used elsewhere (check: it's only used for the deploy panel, so remove it).

- [ ] **Step 2: Remove unused SlidePanel import from Dashboard**

In `Dashboard.svelte`, remove:

```javascript
  import SlidePanel from '../components/SlidePanel.svelte'
```

- [ ] **Step 3: Verify build**

Run: `cd /Users/ameen/dev/vazra/simpledeploy && make build`
Expected: Build succeeds.

- [ ] **Step 4: Manual test checklist**

Run the app and test:
- Open Deploy App panel, verify step indicator shows 3 steps
- Enter invalid name (e.g. `!!!`), verify error appears
- Enter valid name, paste valid compose YAML, verify auto-validation
- Click Next, verify service summary shows parsed services
- Click Back, verify name and compose preserved
- Open routing accordion, fill in domain/port
- Click Deploy, verify log stream appears
- Verify panel stays open during deploy
- Try closing during deploy, verify confirmation dialog

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/DeployWizard.svelte ui/src/routes/Dashboard.svelte
git commit -m "feat(ui): deploy wizard integration polish, SlidePanel moved into wizard"
```
