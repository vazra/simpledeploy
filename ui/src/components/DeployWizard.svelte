<script>
  import Button from './Button.svelte'
  import YamlEditor from './YamlEditor.svelte'
  import { api } from '../lib/api.js'

  let { onclose = () => {}, onComplete = () => {} } = $props()

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
    {#if step === 1}
      <Button size="sm" disabled={!appName.trim() || !!nameError || !composeValid} onclick={enterStep2}>Next</Button>
    {:else if step < 3}
      <Button size="sm" onclick={() => step++}>Next</Button>
    {/if}
  </div>
</div>
