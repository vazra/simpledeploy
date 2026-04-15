<script>
  import FormModal from './FormModal.svelte'
  import ScheduleBuilder from './ScheduleBuilder.svelte'
  import Button from './Button.svelte'
  import Badge from './Badge.svelte'
  import { api } from '../lib/api.js'

  let { open = false, slug = '', onclose = () => {}, oncreated = () => {} } = $props()

  let step = $state(1)
  let creating = $state(false)
  let detecting = $state(false)
  let strategies = $state([])

  let testingS3 = $state(false)
  let s3TestResult = $state(null)

  // Form state
  let selectedStrategy = $state('')
  let selectedTarget = $state('local')
  let cronExpr = $state('0 2 * * *')
  let retentionCount = $state(7)

  // S3 config
  let s3 = $state({
    endpoint: '',
    bucket: '',
    prefix: '',
    access_key: '',
    secret_key: '',
    region: 'us-east-1',
  })

  $effect(() => {
    if (open && slug) {
      resetState()
      loadDetection()
    }
  })

  function resetState() {
    step = 1
    creating = false
    detecting = false
    strategies = []
    testingS3 = false
    s3TestResult = null
    selectedStrategy = ''
    selectedTarget = 'local'
    cronExpr = '0 2 * * *'
    retentionCount = 7
    s3 = { endpoint: '', bucket: '', prefix: '', access_key: '', secret_key: '', region: 'us-east-1' }
  }

  async function loadDetection() {
    detecting = true
    const res = await api.detectStrategies(slug)
    detecting = false
    if (res.data?.strategies) {
      strategies = res.data.strategies
      const first = strategies.find(s => s.available)
      if (first) selectedStrategy = first.type
    }
  }

  function canProceed() {
    if (step === 1) return !!selectedStrategy
    if (step === 2) {
      if (selectedTarget === 'local') return true
      return !!(s3.bucket && s3.access_key && s3.secret_key)
    }
    if (step === 3) return !!cronExpr
    return true
  }

  async function testS3Connection() {
    testingS3 = true
    s3TestResult = null
    const res = await api.testS3({ ...s3 })
    testingS3 = false
    if (res.error) {
      s3TestResult = { ok: false, message: res.error }
    } else {
      s3TestResult = { ok: true, message: 'Connection successful' }
    }
  }

  async function createBackup() {
    creating = true
    const cfg = {
      Strategy: selectedStrategy,
      Target: selectedTarget,
      ScheduleCron: cronExpr,
      RetentionCount: retentionCount,
      TargetConfigJSON: selectedTarget === 's3' ? JSON.stringify(s3) : '',
    }
    const res = await api.createBackupConfig(slug, cfg)
    creating = false
    if (!res.error) {
      oncreated()
      onclose()
    }
  }

  function strategyLabel(type) {
    if (type === 'postgres') return 'Database (PostgreSQL)'
    if (type === 'volume') return 'Files & Volumes'
    return type
  }

  const inputClass = 'w-full bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50'
  const cardClass = 'w-full text-left p-4 rounded-xl border transition-colors'
  const selectedCardClass = 'border-accent bg-accent/5'
  const unselectedCardClass = 'border-border/50 bg-surface-3/30 hover:border-border'
</script>

<FormModal {open} title="Configure Backup" onclose={onclose}>
  <!-- Progress indicator -->
  <div class="flex items-center justify-center mb-6">
    {#each [1, 2, 3, 4] as num, i}
      {#if i > 0}
        <div class="w-8 h-px {step > i ? 'bg-accent' : 'bg-border/50'} mx-1"></div>
      {/if}
      <div class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-medium
        {step === num ? 'bg-accent text-white' : step > num ? 'bg-accent/20 text-accent' : 'bg-surface-3 text-text-muted'}">
        {#if step > num}
          <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        {:else}
          {num}
        {/if}
      </div>
    {/each}
  </div>

  <!-- Step 1: What to Back Up -->
  {#if step === 1}
    <div class="space-y-3">
      <div class="mb-4">
        <h4 class="text-sm font-semibold text-text-primary">What do you want to back up?</h4>
        <p class="text-xs text-text-muted mt-0.5">Select the type of data to protect.</p>
      </div>

      {#if detecting}
        <p class="text-sm text-text-muted py-4 text-center">Detecting available strategies...</p>
      {:else if strategies.length === 0}
        <p class="text-sm text-text-muted py-4 text-center">No backup strategies detected for this app.</p>
      {:else}
        {#each strategies as strategy}
          <button
            type="button"
            disabled={!strategy.available}
            onclick={() => strategy.available && (selectedStrategy = strategy.type)}
            class="{cardClass} {selectedStrategy === strategy.type ? selectedCardClass : unselectedCardClass} {!strategy.available ? 'opacity-50 cursor-not-allowed' : ''}"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="flex-1">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-sm font-medium text-text-primary">{strategy.label || strategyLabel(strategy.type)}</span>
                  {#if strategy.available}
                    <Badge variant="success">Detected</Badge>
                  {/if}
                </div>
                <p class="text-xs text-text-muted">{strategy.description || ''}</p>
                {#if strategy.containers?.length > 0}
                  <p class="text-xs text-text-muted mt-1">Containers: <span class="font-mono text-text-secondary">{strategy.containers.join(', ')}</span></p>
                {/if}
                {#if strategy.volumes && strategy.volumes.length > 0}
                  <p class="text-xs text-text-muted mt-1">Volumes: <span class="font-mono text-text-secondary">{strategy.volumes.join(', ')}</span></p>
                {/if}
                {#if !strategy.available}
                  <p class="text-xs text-warning mt-1">Not available for this app</p>
                {/if}
              </div>
              {#if selectedStrategy === strategy.type}
                <div class="text-accent shrink-0 mt-0.5">
                  <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                  </svg>
                </div>
              {/if}
            </div>
          </button>
        {/each}
      {/if}
    </div>

  <!-- Step 2: Where to Store -->
  {:else if step === 2}
    <div class="space-y-3">
      <div class="mb-4">
        <h4 class="text-sm font-semibold text-text-primary">Where should backups be stored?</h4>
        <p class="text-xs text-text-muted mt-0.5">Choose a storage destination.</p>
      </div>

      <button
        type="button"
        onclick={() => selectedTarget = 'local'}
        class="{cardClass} {selectedTarget === 'local' ? selectedCardClass : unselectedCardClass}"
      >
        <div class="flex items-center gap-3">
          <div class="w-8 h-8 rounded-lg bg-surface-3 flex items-center justify-center shrink-0">
            <svg class="w-4 h-4 text-text-secondary" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
            </svg>
          </div>
          <div>
            <p class="text-sm font-medium text-text-primary">Local Storage</p>
            <p class="text-xs text-text-muted">Save backups on this server's filesystem</p>
          </div>
          {#if selectedTarget === 'local'}
            <div class="text-accent ml-auto">
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
              </svg>
            </div>
          {/if}
        </div>
      </button>

      <button
        type="button"
        onclick={() => selectedTarget = 's3'}
        class="{cardClass} {selectedTarget === 's3' ? selectedCardClass : unselectedCardClass}"
      >
        <div class="flex items-center gap-3">
          <div class="w-8 h-8 rounded-lg bg-surface-3 flex items-center justify-center shrink-0">
            <svg class="w-4 h-4 text-text-secondary" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3 15a4 4 0 004 4h9a5 5 0 10-.1-9.999 5.002 5.002 0 10-9.78 2.096A4.001 4.001 0 003 15z" />
            </svg>
          </div>
          <div>
            <p class="text-sm font-medium text-text-primary">S3-Compatible Storage</p>
            <p class="text-xs text-text-muted">AWS S3, Backblaze B2, MinIO, and more</p>
          </div>
          {#if selectedTarget === 's3'}
            <div class="text-accent ml-auto">
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
              </svg>
            </div>
          {/if}
        </div>
      </button>

      {#if selectedTarget === 's3'}
        <div class="mt-4 p-4 bg-surface-3/30 rounded-xl border border-border/50 space-y-3">
          <div>
            <label class="block text-xs font-medium text-text-secondary mb-1">Endpoint</label>
            <input type="text" class={inputClass} placeholder="https://s3.amazonaws.com" bind:value={s3.endpoint} />
            <p class="text-xs text-text-muted mt-1">Leave blank for AWS S3</p>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs font-medium text-text-secondary mb-1">
                Bucket <span class="text-danger">*</span>
              </label>
              <input type="text" class={inputClass} placeholder="my-backups" bind:value={s3.bucket} />
            </div>
            <div>
              <label class="block text-xs font-medium text-text-secondary mb-1">Prefix</label>
              <input type="text" class={inputClass} placeholder="backups/" bind:value={s3.prefix} />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs font-medium text-text-secondary mb-1">
                Access Key <span class="text-danger">*</span>
              </label>
              <input type="text" class={inputClass} placeholder="AKIAIOSFODNN7EXAMPLE" bind:value={s3.access_key} />
            </div>
            <div>
              <label class="block text-xs font-medium text-text-secondary mb-1">
                Secret Key <span class="text-danger">*</span>
              </label>
              <input type="password" class={inputClass} placeholder="••••••••" bind:value={s3.secret_key} />
            </div>
          </div>
          <div>
            <label class="block text-xs font-medium text-text-secondary mb-1">Region</label>
            <input type="text" class={inputClass} placeholder="us-east-1" bind:value={s3.region} />
          </div>

          <div class="flex items-center gap-3 pt-1">
            <Button size="sm" variant="secondary" loading={testingS3} onclick={testS3Connection}>
              Test Connection
            </Button>
            {#if s3TestResult}
              <span class="text-xs {s3TestResult.ok ? 'text-success' : 'text-danger'}">
                {s3TestResult.message}
              </span>
            {/if}
          </div>
        </div>
      {/if}
    </div>

  <!-- Step 3: Schedule -->
  {:else if step === 3}
    <div class="space-y-4">
      <div class="mb-4">
        <h4 class="text-sm font-semibold text-text-primary">When should backups run?</h4>
        <p class="text-xs text-text-muted mt-0.5">Set the automatic backup schedule.</p>
      </div>
      <ScheduleBuilder value={cronExpr} onchange={(c) => cronExpr = c} />
    </div>

  <!-- Step 4: Retention + Summary -->
  {:else if step === 4}
    <div class="space-y-5">
      <div>
        <h4 class="text-sm font-semibold text-text-primary mb-1">How many backups to keep?</h4>
        <p class="text-xs text-text-muted mb-3">Older backups are automatically deleted when the limit is reached.</p>
        <div class="flex items-center gap-3">
          <input
            type="number"
            min="1"
            class="w-24 bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50"
            bind:value={retentionCount}
          />
          <span class="text-sm text-text-muted">backups</span>
        </div>
        <p class="text-xs text-text-muted mt-2">The {retentionCount} most recent backup{retentionCount !== 1 ? 's' : ''} will be kept. Older ones are removed automatically.</p>
      </div>

      <!-- Summary -->
      <div class="bg-surface-3/30 border border-border/50 rounded-xl p-4 space-y-3">
        <h5 class="text-xs font-semibold text-text-muted uppercase tracking-wider">Summary</h5>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <p class="text-xs text-text-muted mb-0.5">Backup type</p>
            <p class="text-sm font-medium text-text-primary">{strategyLabel(selectedStrategy)}</p>
          </div>
          <div>
            <p class="text-xs text-text-muted mb-0.5">Destination</p>
            <p class="text-sm font-medium text-text-primary">
              {selectedTarget === 'local' ? 'Local Storage' : 'S3-Compatible'}
              {#if selectedTarget === 's3' && s3.bucket}
                <span class="text-text-muted font-normal"> ({s3.bucket})</span>
              {/if}
            </p>
          </div>
          <div>
            <p class="text-xs text-text-muted mb-0.5">Schedule</p>
            <p class="text-sm font-medium text-text-primary font-mono">{cronExpr}</p>
          </div>
          <div>
            <p class="text-xs text-text-muted mb-0.5">Retention</p>
            <p class="text-sm font-medium text-text-primary">{retentionCount} backup{retentionCount !== 1 ? 's' : ''}</p>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- Navigation -->
  <div class="flex justify-between mt-6 pt-4 border-t border-border/30">
    {#if step > 1}
      <Button variant="secondary" size="sm" onclick={() => step--}>Back</Button>
    {:else}
      <div></div>
    {/if}

    {#if step < 4}
      <Button size="sm" disabled={!canProceed()} onclick={() => step++}>Next</Button>
    {:else}
      <Button size="sm" loading={creating} disabled={!canProceed() || creating} onclick={createBackup}>
        Create Backup
      </Button>
    {/if}
  </div>
</FormModal>
