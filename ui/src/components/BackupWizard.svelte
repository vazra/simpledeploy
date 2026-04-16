<script>
  import FormModal from './FormModal.svelte'
  import ScheduleBuilder from './ScheduleBuilder.svelte'
  import Button from './Button.svelte'
  import Badge from './Badge.svelte'
  import { api } from '../lib/api.js'

  let { open = false, slug = '', editConfig = null, onclose = () => {}, oncreated = () => {} } = $props()

  const TOTAL_STEPS = 6
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
  let retentionMode = $state('count')
  let retentionDays = $state(30)
  let verifyUpload = $state(false)
  let preHooks = $state([])
  let postHooks = $state([])
  let selectedPaths = $state([])

  // Hook toggles for smart suggestions
  let stopDuringBackup = $state(false)
  let redisFlush = $state(false)
  let customPreHook = $state({ service: '', command: '' })
  let customPostHook = $state({ service: '', command: '' })
  let showCustomPre = $state(false)
  let showCustomPost = $state(false)

  // S3 config
  let s3 = $state({
    endpoint: '',
    bucket: '',
    prefix: '',
    access_key: '',
    secret_key: '',
    region: 'us-east-1',
  })

  // Detected paths for volume/sqlite strategies
  let detectedPaths = $state([])

  $effect(() => {
    if (open && slug) {
      resetState()
      if (editConfig) {
        populateFromConfig(editConfig)
      }
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
    retentionMode = 'count'
    retentionDays = 30
    verifyUpload = false
    preHooks = []
    postHooks = []
    selectedPaths = []
    detectedPaths = []
    stopDuringBackup = false
    redisFlush = false
    customPreHook = { service: '', command: '' }
    customPostHook = { service: '', command: '' }
    showCustomPre = false
    showCustomPost = false
    s3 = { endpoint: '', bucket: '', prefix: '', access_key: '', secret_key: '', region: 'us-east-1' }
  }

  function populateFromConfig(cfg) {
    selectedStrategy = cfg.strategy || ''
    selectedTarget = cfg.target || 'local'
    cronExpr = cfg.schedule_cron || '0 2 * * *'
    retentionCount = cfg.retention_count || 7
    retentionMode = cfg.retention_mode || 'count'
    retentionDays = cfg.retention_days || 30
    verifyUpload = cfg.verify_upload || false

    if (cfg.target === 's3' && cfg.target_config_json) {
      try {
        const parsed = typeof cfg.target_config_json === 'string' ? JSON.parse(cfg.target_config_json) : cfg.target_config_json
        s3 = { ...s3, ...parsed }
      } catch {}
    }

    try {
      if (cfg.pre_hooks) preHooks = typeof cfg.pre_hooks === 'string' ? JSON.parse(cfg.pre_hooks) : cfg.pre_hooks
      if (cfg.post_hooks) postHooks = typeof cfg.post_hooks === 'string' ? JSON.parse(cfg.post_hooks) : cfg.post_hooks
      if (cfg.paths) selectedPaths = typeof cfg.paths === 'string' ? JSON.parse(cfg.paths) : cfg.paths
    } catch {}
  }

  async function loadDetection() {
    detecting = true
    const res = await api.detectStrategies(slug)
    detecting = false
    if (res.data?.strategies) {
      strategies = res.data.strategies
      // Set detected paths for volume/sqlite
      const activeStrat = strategies.find(s => s.strategy_type === selectedStrategy || s.type === selectedStrategy)
      if (activeStrat?.volumes) {
        detectedPaths = activeStrat.volumes || []
        if (selectedPaths.length === 0) selectedPaths = [...detectedPaths]
      }
      if (!selectedStrategy) {
        const first = strategies.find(s => s.available)
        if (first) selectedStrategy = first.strategy_type || first.type
      }
    }
  }

  function getStrategyType(s) {
    return s.strategy_type || s.type
  }

  function getStrategyLabel(s) {
    return s.label || strategyLabel(getStrategyType(s))
  }

  // Update detected paths when strategy changes
  $effect(() => {
    if (selectedStrategy && strategies.length > 0) {
      const strat = strategies.find(s => getStrategyType(s) === selectedStrategy)
      if (strat?.volumes) {
        detectedPaths = strat.volumes
        if (selectedPaths.length === 0) selectedPaths = [...detectedPaths]
      } else {
        detectedPaths = []
      }
    }
  })

  function canProceed() {
    if (step === 1) return !!selectedStrategy
    if (step === 2) {
      if (selectedTarget === 'local') return true
      return !!(s3.bucket && s3.access_key && s3.secret_key)
    }
    if (step === 3) return !!cronExpr
    if (step === 4) return true
    if (step === 5) return retentionMode === 'count' ? retentionCount > 0 : retentionDays > 0
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

  function buildHooks() {
    let pre = [...preHooks]
    let post = [...postHooks]

    if (stopDuringBackup) {
      const strat = strategies.find(s => getStrategyType(s) === selectedStrategy)
      const svc = strat?.services?.[0] || strat?.containers?.[0] || ''
      if (svc) {
        pre = [{ type: 'stop_container', service: svc }, ...pre]
        post = [...post, { type: 'start_container', service: svc }]
      }
    }
    if (redisFlush) {
      const strat = strategies.find(s => getStrategyType(s) === selectedStrategy)
      const svc = strat?.services?.[0] || strat?.containers?.[0] || ''
      if (svc) {
        pre = [{ type: 'exec', service: svc, command: 'redis-cli BGSAVE' }, ...pre]
      }
    }

    return { pre, post }
  }

  async function createBackup() {
    creating = true
    const hooks = buildHooks()
    const cfg = {
      strategy: selectedStrategy,
      target: selectedTarget,
      schedule_cron: cronExpr,
      retention_count: retentionMode === 'count' ? retentionCount : 0,
      retention_mode: retentionMode,
      retention_days: retentionMode === 'days' ? retentionDays : 0,
      verify_upload: verifyUpload,
      target_config_json: selectedTarget === 's3' ? JSON.stringify(s3) : '',
      pre_hooks: JSON.stringify(hooks.pre),
      post_hooks: JSON.stringify(hooks.post),
      paths: selectedPaths.length > 0 ? JSON.stringify(selectedPaths) : '',
    }

    let res
    if (editConfig) {
      res = await api.updateBackupConfig(editConfig.id, cfg)
    } else {
      res = await api.createBackupConfig(slug, cfg)
    }
    creating = false
    if (!res.error) {
      oncreated()
      onclose()
    }
  }

  function addCustomPreHook() {
    if (customPreHook.service && customPreHook.command) {
      preHooks = [...preHooks, { type: 'exec', service: customPreHook.service, command: customPreHook.command }]
      customPreHook = { service: '', command: '' }
    }
  }

  function addCustomPostHook() {
    if (customPostHook.service && customPostHook.command) {
      postHooks = [...postHooks, { type: 'exec', service: customPostHook.service, command: customPostHook.command }]
      customPostHook = { service: '', command: '' }
    }
  }

  function removePreHook(i) {
    preHooks = preHooks.filter((_, idx) => idx !== i)
  }

  function removePostHook(i) {
    postHooks = postHooks.filter((_, idx) => idx !== i)
  }

  function togglePath(path) {
    if (selectedPaths.includes(path)) {
      selectedPaths = selectedPaths.filter(p => p !== path)
    } else {
      selectedPaths = [...selectedPaths, path]
    }
  }

  function strategyLabel(type) {
    const labels = {
      postgres: 'Database (PostgreSQL)',
      mysql: 'Database (MySQL)',
      redis: 'Redis',
      volume: 'Files & Volumes',
      sqlite: 'SQLite Database',
      mongo: 'Database (MongoDB)',
    }
    return labels[type] || type
  }

  function retentionSummary() {
    if (retentionMode === 'count') return `Keep last ${retentionCount} backup${retentionCount !== 1 ? 's' : ''}`
    return `Keep for ${retentionDays} day${retentionDays !== 1 ? 's' : ''}`
  }

  function hooksSummary() {
    const hooks = buildHooks()
    const total = hooks.pre.length + hooks.post.length
    if (total === 0) return 'None'
    return `${hooks.pre.length} pre, ${hooks.post.length} post`
  }

  const inputClass = 'w-full bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 focus:ring-accent/50'
  const cardClass = 'w-full text-left p-4 rounded-xl border transition-colors'
  const selectedCardClass = 'border-accent bg-accent/5'
  const unselectedCardClass = 'border-border/50 bg-surface-3/30 hover:border-border'

  let isVolumeLike = $derived(selectedStrategy === 'volume' || selectedStrategy === 'sqlite')
  let isRedis = $derived(selectedStrategy === 'redis')
  let showHookSuggestions = $derived(isVolumeLike || isRedis)
</script>

<FormModal {open} title={editConfig ? 'Edit Backup Config' : 'Configure Backup'} onclose={onclose}>
  <!-- Progress indicator -->
  <div class="flex items-center justify-center mb-6">
    {#each [1, 2, 3, 4, 5, 6] as num, i}
      {#if i > 0}
        <div class="w-6 h-px {step > i ? 'bg-accent' : 'bg-border/50'} mx-0.5"></div>
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
            onclick={() => strategy.available && (selectedStrategy = getStrategyType(strategy))}
            class="{cardClass} {selectedStrategy === getStrategyType(strategy) ? selectedCardClass : unselectedCardClass} {!strategy.available ? 'opacity-50 cursor-not-allowed' : ''}"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="flex-1">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-sm font-medium text-text-primary">{getStrategyLabel(strategy)}</span>
                  {#if strategy.available}
                    <Badge variant="success">Detected</Badge>
                  {/if}
                </div>
                <p class="text-xs text-text-muted">{strategy.description || ''}</p>
                {#if strategy.services?.length > 0}
                  <p class="text-xs text-text-muted mt-1">Services: <span class="font-mono text-text-secondary">{strategy.services.join(', ')}</span></p>
                {:else if strategy.containers?.length > 0}
                  <p class="text-xs text-text-muted mt-1">Containers: <span class="font-mono text-text-secondary">{strategy.containers.join(', ')}</span></p>
                {/if}
                {#if strategy.volumes && strategy.volumes.length > 0}
                  <p class="text-xs text-text-muted mt-1">Volumes: <span class="font-mono text-text-secondary">{strategy.volumes.join(', ')}</span></p>
                {/if}
                {#if !strategy.available}
                  <p class="text-xs text-warning mt-1">Not available for this app</p>
                {/if}
              </div>
              {#if selectedStrategy === getStrategyType(strategy)}
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

  <!-- Step 4: Hooks -->
  {:else if step === 4}
    <div class="space-y-4">
      <div class="mb-4">
        <h4 class="text-sm font-semibold text-text-primary">Pre/post backup hooks</h4>
        <p class="text-xs text-text-muted mt-0.5">Optional commands to run before or after the backup.</p>
      </div>

      {#if showHookSuggestions}
        <div class="space-y-3">
          {#if isVolumeLike}
            <label class="flex items-start gap-3 p-3 rounded-xl border border-border/50 bg-surface-3/30 cursor-pointer hover:border-border transition-colors">
              <input type="checkbox" bind:checked={stopDuringBackup} class="mt-0.5 accent-accent" />
              <div>
                <p class="text-sm font-medium text-text-primary">Stop container during backup</p>
                <p class="text-xs text-text-muted">Ensures data consistency by stopping the service before backing up files, then restarting after.</p>
              </div>
            </label>
          {/if}
          {#if isRedis}
            <label class="flex items-start gap-3 p-3 rounded-xl border border-border/50 bg-surface-3/30 cursor-pointer hover:border-border transition-colors">
              <input type="checkbox" bind:checked={redisFlush} class="mt-0.5 accent-accent" />
              <div>
                <p class="text-sm font-medium text-text-primary">Flush to disk before backup</p>
                <p class="text-xs text-text-muted">Runs <code class="text-xs font-mono bg-surface-3 px-1 rounded">redis-cli BGSAVE</code> to ensure all data is written to disk.</p>
              </div>
            </label>
          {/if}
        </div>
      {/if}

      <!-- Custom pre-hooks -->
      <div class="space-y-2">
        <button
          type="button"
          onclick={() => showCustomPre = !showCustomPre}
          class="flex items-center gap-2 text-xs font-medium text-text-secondary hover:text-text-primary transition-colors"
        >
          <svg class="w-3.5 h-3.5 transition-transform {showCustomPre ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          Custom pre-backup command
        </button>

        {#if showCustomPre}
          <div class="p-3 bg-surface-3/30 rounded-xl border border-border/50 space-y-2">
            <div class="grid grid-cols-3 gap-2">
              <input type="text" class={inputClass} placeholder="Service name" bind:value={customPreHook.service} />
              <input type="text" class="col-span-2 {inputClass}" placeholder="Command to run" bind:value={customPreHook.command} />
            </div>
            <Button size="sm" variant="secondary" onclick={addCustomPreHook} disabled={!customPreHook.service || !customPreHook.command}>Add</Button>
          </div>
        {/if}

        {#each preHooks as hook, i}
          <div class="flex items-center gap-2 px-3 py-2 bg-surface-3/20 rounded-lg text-xs">
            <span class="font-mono text-text-secondary">{hook.service}: {hook.command}</span>
            <button type="button" onclick={() => removePreHook(i)} class="ml-auto text-text-muted hover:text-danger transition-colors">
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        {/each}
      </div>

      <!-- Custom post-hooks -->
      <div class="space-y-2">
        <button
          type="button"
          onclick={() => showCustomPost = !showCustomPost}
          class="flex items-center gap-2 text-xs font-medium text-text-secondary hover:text-text-primary transition-colors"
        >
          <svg class="w-3.5 h-3.5 transition-transform {showCustomPost ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          Custom post-backup command
        </button>

        {#if showCustomPost}
          <div class="p-3 bg-surface-3/30 rounded-xl border border-border/50 space-y-2">
            <div class="grid grid-cols-3 gap-2">
              <input type="text" class={inputClass} placeholder="Service name" bind:value={customPostHook.service} />
              <input type="text" class="col-span-2 {inputClass}" placeholder="Command to run" bind:value={customPostHook.command} />
            </div>
            <Button size="sm" variant="secondary" onclick={addCustomPostHook} disabled={!customPostHook.service || !customPostHook.command}>Add</Button>
          </div>
        {/if}

        {#each postHooks as hook, i}
          <div class="flex items-center gap-2 px-3 py-2 bg-surface-3/20 rounded-lg text-xs">
            <span class="font-mono text-text-secondary">{hook.service}: {hook.command}</span>
            <button type="button" onclick={() => removePostHook(i)} class="ml-auto text-text-muted hover:text-danger transition-colors">
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        {/each}
      </div>

      {#if showCustomPre || showCustomPost || preHooks.length > 0 || postHooks.length > 0}
        <p class="text-xs text-warning flex items-center gap-1.5">
          <svg class="w-3.5 h-3.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          Custom commands run inside the container as root.
        </p>
      {/if}
    </div>

  <!-- Step 5: Retention & Verification -->
  {:else if step === 5}
    <div class="space-y-5">
      <div>
        <h4 class="text-sm font-semibold text-text-primary mb-1">Retention policy</h4>
        <p class="text-xs text-text-muted mb-3">How long to keep backups before automatic cleanup.</p>

        <div class="flex items-center gap-2 mb-3">
          <button
            type="button"
            onclick={() => retentionMode = 'count'}
            class="px-3 py-1.5 text-xs rounded-lg border transition-colors {retentionMode === 'count' ? 'border-accent bg-accent/10 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
          >Keep last N backups</button>
          <button
            type="button"
            onclick={() => retentionMode = 'days'}
            class="px-3 py-1.5 text-xs rounded-lg border transition-colors {retentionMode === 'days' ? 'border-accent bg-accent/10 text-accent' : 'border-border/50 text-text-muted hover:text-text-primary'}"
          >Keep for N days</button>
        </div>

        <div class="flex items-center gap-3">
          {#if retentionMode === 'count'}
            <input
              type="number"
              min="1"
              class="w-24 bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50"
              bind:value={retentionCount}
            />
            <span class="text-sm text-text-muted">backups</span>
          {:else}
            <input
              type="number"
              min="1"
              class="w-24 bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/50"
              bind:value={retentionDays}
            />
            <span class="text-sm text-text-muted">days</span>
          {/if}
        </div>
        <p class="text-xs text-text-muted mt-2">
          {#if retentionMode === 'count'}
            The {retentionCount} most recent backup{retentionCount !== 1 ? 's' : ''} will be kept. Older ones are removed automatically.
          {:else}
            Backups older than {retentionDays} day{retentionDays !== 1 ? 's' : ''} are removed automatically.
          {/if}
        </p>
      </div>

      <!-- Verify upload -->
      <label class="flex items-start gap-3 p-3 rounded-xl border border-border/50 bg-surface-3/30 cursor-pointer hover:border-border transition-colors">
        <input type="checkbox" bind:checked={verifyUpload} class="mt-0.5 accent-accent" />
        <div>
          <p class="text-sm font-medium text-text-primary">Verify backup after upload</p>
          <p class="text-xs text-text-muted">Re-download and compare checksums to ensure integrity. Slightly slower but more reliable.</p>
        </div>
      </label>

      <!-- Path selection for volume/sqlite -->
      {#if isVolumeLike && detectedPaths.length > 0}
        <div>
          <h4 class="text-sm font-semibold text-text-primary mb-1">Paths to back up</h4>
          <p class="text-xs text-text-muted mb-3">Select which volumes or paths to include.</p>
          <div class="space-y-2">
            {#each detectedPaths as path}
              <label class="flex items-center gap-3 px-3 py-2 rounded-lg border border-border/50 bg-surface-3/20 cursor-pointer hover:border-border transition-colors">
                <input
                  type="checkbox"
                  checked={selectedPaths.includes(path)}
                  onchange={() => togglePath(path)}
                  class="accent-accent"
                />
                <span class="text-sm font-mono text-text-secondary">{path}</span>
              </label>
            {/each}
          </div>
        </div>
      {/if}
    </div>

  <!-- Step 6: Summary -->
  {:else if step === 6}
    <div class="space-y-5">
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
            <p class="text-sm font-medium text-text-primary">{retentionSummary()}</p>
          </div>
          <div>
            <p class="text-xs text-text-muted mb-0.5">Hooks</p>
            <p class="text-sm font-medium text-text-primary">{hooksSummary()}</p>
          </div>
          <div>
            <p class="text-xs text-text-muted mb-0.5">Verify upload</p>
            <p class="text-sm font-medium text-text-primary">{verifyUpload ? 'Yes' : 'No'}</p>
          </div>
          {#if selectedPaths.length > 0}
            <div class="col-span-2">
              <p class="text-xs text-text-muted mb-0.5">Paths</p>
              <p class="text-sm font-mono text-text-secondary">{selectedPaths.join(', ')}</p>
            </div>
          {/if}
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

    {#if step < TOTAL_STEPS}
      <Button size="sm" disabled={!canProceed()} onclick={() => step++}>Next</Button>
    {:else}
      <Button size="sm" loading={creating} disabled={!canProceed() || creating} onclick={createBackup}>
        {editConfig ? 'Save Changes' : 'Create Backup'}
      </Button>
    {/if}
  </div>
</FormModal>
