<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import Modal from '../components/Modal.svelte'
  import FormModal from '../components/FormModal.svelte'
  import { api } from '../lib/api.js'
  import { timeAgo } from '../lib/format.js'
  import { realtime } from '../lib/stores/realtime.svelte.js'

  let webhooks = $state([])
  let rules = $state([])
  let history = $state([])
  let apps = $state([])
  let loading = $state(true)

  // Delete confirmation
  let deleteTarget = $state(null)

  // Webhook modal
  let showWebhookModal = $state(false)
  let editingWebhook = $state(null)
  let whName = $state('')
  let whType = $state('slack')
  let whUrl = $state('')
  let whTemplate = $state('')
  let whHeaders = $state('')
  let whAdvancedOpen = $state(false)
  let whSaving = $state(false)
  let whTesting = $state(false)
  // Telegram URL builder
  let tgBotToken = $state('')
  let tgChatId = $state('')
  let tgUseBuilder = $state(true)

  // Rule modal
  let showRuleModal = $state(false)
  let editingRule = $state(null)
  let rApp = $state('')
  let rMetric = $state('cpu_pct')
  let rOp = $state('>')
  let rThreshold = $state(80)
  let rDuration = $state(60)
  let rWebhook = $state('')
  let rEnabled = $state(true)
  let rSaving = $state(false)

  const webhookTypes = ['slack', 'discord', 'telegram', 'custom']

  const typeLabels = { slack: 'Slack', discord: 'Discord', telegram: 'Telegram', custom: 'Custom' }

  const urlPlaceholders = {
    slack: 'https://hooks.slack.com/services/T.../B.../xxx',
    discord: 'https://discord.com/api/webhooks/123/abc',
    telegram: 'https://api.telegram.org/bot<token>/sendMessage',
    custom: 'https://example.com/webhook',
  }

  const urlHelp = {
    slack: 'Create an Incoming Webhook in your Slack workspace settings',
    discord: 'Go to Server Settings > Integrations > Webhooks',
    telegram: 'Use BotFather to create a bot, then use the bot token and chat ID',
    custom: 'Any URL that accepts POST requests with JSON body',
  }

  const metricLabels = {
    cpu_pct: 'CPU Usage',
    mem_pct: 'Memory %',
    mem_bytes: 'Memory Used',
  }

  const metricHelp = {
    cpu_pct: 'Percentage of CPU used by the container. 100% = one full core.',
    mem_pct: 'Memory used as a percentage of the container memory limit.',
    mem_bytes: 'Absolute memory used by the container in MB or GB.',
  }

  const durationPresets = [
    { label: '30s', value: 30 },
    { label: '1m', value: 60 },
    { label: '5m', value: 300 },
    { label: '10m', value: 600 },
    { label: '30m', value: 1800 },
    { label: '1h', value: 3600 },
  ]

  let thresholdUnit = $state('MB')

  const sampleData = {
    AppName: 'my-app', Metric: 'cpu_pct', MetricDisplay: 'CPU',
    Value: 92.5, ValueDisplay: '92.5%', Threshold: 80, ThresholdDisplay: '80.0%',
    Operator: '>', Status: 'firing',
  }

  function builtinPreview(type) {
    if (type === 'custom') {
      return JSON.stringify({ app: sampleData.AppName, metric: sampleData.Metric, value: 92.50, threshold: 80.00, status: sampleData.Status }, null, 2)
    }
    if (type === 'telegram') {
      return `[${sampleData.Status}] ${sampleData.AppName}\n${sampleData.MetricDisplay} ${sampleData.Operator} ${sampleData.ThresholdDisplay} (current: ${sampleData.ValueDisplay})`
    }
    return `[${sampleData.Status}] ${sampleData.AppName} - ${sampleData.MetricDisplay} ${sampleData.Operator} ${sampleData.ThresholdDisplay} (current: ${sampleData.ValueDisplay})`
  }

  function renderTemplate(tpl) {
    return tpl
      .replace(/\{\{\.AppName\}\}/g, sampleData.AppName)
      .replace(/\{\{\.Metric\}\}/g, sampleData.Metric)
      .replace(/\{\{\.MetricDisplay\}\}/g, sampleData.MetricDisplay)
      .replace(/\{\{\.Value\}\}/g, String(sampleData.Value))
      .replace(/\{\{\.ValueDisplay\}\}/g, sampleData.ValueDisplay)
      .replace(/\{\{\.Threshold\}\}/g, String(sampleData.Threshold))
      .replace(/\{\{\.ThresholdDisplay\}\}/g, sampleData.ThresholdDisplay)
      .replace(/\{\{\.Operator\}\}/g, sampleData.Operator)
      .replace(/\{\{\.Status\}\}/g, sampleData.Status)
      .replace(/\{\{\.FiredAt\}\}/g, new Date().toISOString())
  }

  let previewText = $derived(whTemplate.trim() ? renderTemplate(whTemplate) : builtinPreview(whType))

  function webhookName(id) {
    const w = webhooks.find(w => w.id === id)
    return w ? w.name : `#${id}`
  }

  function formatMetricValue(metric, value) {
    if (value == null) return '-'
    if (metric === 'mem_bytes') {
      if (value >= 1 << 30) return `${(value / (1 << 30)).toFixed(1)} GB`
      if (value >= 1 << 20) return `${(value / (1 << 20)).toFixed(1)} MB`
      if (value >= 1 << 10) return `${(value / (1 << 10)).toFixed(1)} KB`
      return `${value.toFixed(0)} B`
    }
    if (metric === 'cpu_pct' || metric === 'mem_pct') return `${value.toFixed(1)}%`
    return value.toFixed(1)
  }

  function ruleName(r) {
    const metric = metricLabels[r.metric] || r.metric
    const threshold = formatMetricValue(r.metric, r.threshold)
    const app = r.app_slug || 'All Apps'
    return `${metric} ${r.operator} ${threshold} - ${app}`
  }

  function historyName(h) {
    if (h.metric) return ruleName(h)
    const r = rules.find(r => r.id === h.rule_id)
    if (r) return ruleName(r)
    return `Rule #${h.rule_id}`
  }

  function historyMetric(h) {
    if (h.metric) return h.metric
    const r = rules.find(r => r.id === h.rule_id)
    return r ? r.metric : null
  }

  function formatDuration(secs) {
    if (secs >= 86400) return `${Math.floor(secs / 86400)}d`
    if (secs >= 3600) return `${Math.floor(secs / 3600)}h`
    if (secs >= 60) return `${Math.floor(secs / 60)}m`
    return `${secs}s`
  }

  function humanDuration(secs) {
    if (!secs || secs <= 0) return ''
    const parts = []
    const d = Math.floor(secs / 86400)
    const h = Math.floor((secs % 86400) / 3600)
    const m = Math.floor((secs % 3600) / 60)
    const s = secs % 60
    if (d) parts.push(`${d} day${d > 1 ? 's' : ''}`)
    if (h) parts.push(`${h} hour${h > 1 ? 's' : ''}`)
    if (m) parts.push(`${m} minute${m > 1 ? 's' : ''}`)
    if (s) parts.push(`${s} second${s > 1 ? 's' : ''}`)
    return '= ' + parts.join(', ')
  }


  onMount(() => {
    loadAll()
    return realtime.register('global:alerts', loadAll)
  })

  async function loadAll() {
    loading = true
    const [wRes, rRes, hRes, aRes] = await Promise.all([
      api.listWebhooks(),
      api.listAlertRules(),
      api.alertHistory(),
      api.listApps(),
    ])
    webhooks = wRes.data || []
    rules = rRes.data || []
    history = hRes.data || []
    apps = aRes.data || []
    loading = false
  }

  // Webhook CRUD
  function parseTelegramUrl(url) {
    const m = url.match(/\/bot([^/]+)\/sendMessage\?chat_id=(.+)/)
    return m ? { token: m[1], chatId: m[2] } : null
  }

  function openWebhookCreate() {
    editingWebhook = null
    whName = ''; whType = 'slack'; whUrl = ''; whTemplate = ''; whHeaders = ''; whAdvancedOpen = false
    tgBotToken = ''; tgChatId = ''; tgUseBuilder = true
    showWebhookModal = true
  }

  function openWebhookEdit(w) {
    editingWebhook = w
    whName = w.name; whType = w.type; whUrl = w.url
    whTemplate = w.template_override || ''; whHeaders = w.headers_json || ''
    whAdvancedOpen = !!(w.template_override || w.headers_json)
    if (w.type === 'telegram') {
      const parsed = parseTelegramUrl(w.url)
      if (parsed) {
        tgBotToken = parsed.token; tgChatId = parsed.chatId; tgUseBuilder = true
      } else {
        tgBotToken = ''; tgChatId = ''; tgUseBuilder = false
      }
    } else {
      tgBotToken = ''; tgChatId = ''; tgUseBuilder = true
    }
    showWebhookModal = true
  }

  async function saveWebhook() {
    whSaving = true
    const url = (whType === 'telegram' && tgUseBuilder && tgBotToken && tgChatId)
      ? `https://api.telegram.org/bot${tgBotToken}/sendMessage?chat_id=${tgChatId}`
      : whUrl
    const payload = { name: whName, type: whType, url }
    if (whTemplate.trim()) payload.template_override = whTemplate
    if (whHeaders.trim()) payload.headers_json = whHeaders
    const res = editingWebhook
      ? await api.updateWebhook(editingWebhook.id, payload)
      : await api.createWebhook(payload)
    whSaving = false
    if (!res.error) { showWebhookModal = false; loadAll() }
  }

  async function testWebhook() {
    whTesting = true
    const testUrl = (whType === 'telegram' && tgUseBuilder && tgBotToken && tgChatId)
      ? `https://api.telegram.org/bot${tgBotToken}/sendMessage?chat_id=${tgChatId}`
      : whUrl
    const data = editingWebhook
      ? { webhook_id: editingWebhook.id }
      : { type: whType, url: testUrl, template_override: whTemplate || undefined, headers_json: whHeaders || undefined }
    await api.testWebhook(data)
    whTesting = false
  }

  // Rule CRUD
  function openRuleCreate() {
    editingRule = null
    rApp = ''; rMetric = 'cpu_pct'; rOp = '>'; rThreshold = 80; rDuration = 60; rWebhook = ''; rEnabled = true
    showRuleModal = true
  }

  function openRuleEdit(r) {
    editingRule = r
    rApp = r.app_slug || ''; rMetric = r.metric; rOp = r.operator
    rDuration = r.duration_sec; rWebhook = String(r.webhook_id || ''); rEnabled = r.enabled !== false
    if (r.metric === 'mem_bytes') {
      if (r.threshold >= 1 << 30) { rThreshold = +(r.threshold / (1 << 30)).toFixed(2); thresholdUnit = 'GB' }
      else { rThreshold = +(r.threshold / (1 << 20)).toFixed(2); thresholdUnit = 'MB' }
    } else {
      rThreshold = r.threshold
    }
    showRuleModal = true
  }

  async function saveRule() {
    rSaving = true
    let threshold = +rThreshold
    if (rMetric === 'mem_bytes') {
      threshold = thresholdUnit === 'GB' ? threshold * (1 << 30) : threshold * (1 << 20)
    }
    const payload = {
      app_slug: rApp, metric: rMetric, operator: rOp,
      threshold, duration_sec: +rDuration, webhook_id: +rWebhook, enabled: rEnabled,
    }
    const res = editingRule
      ? await api.updateAlertRule(editingRule.id, payload)
      : await api.createAlertRule(payload)
    rSaving = false
    if (!res.error) { showRuleModal = false; loadAll() }
  }

  // Delete
  async function confirmDelete() {
    if (!deleteTarget) return
    if (deleteTarget.type === 'webhook') {
      await api.deleteWebhook(deleteTarget.id)
    } else if (deleteTarget.type === 'history') {
      await api.clearAlertHistory(deleteTarget.mode)
    } else {
      await api.deleteAlertRule(deleteTarget.id)
    }
    deleteTarget = null
    loadAll()
  }

  let hasResolved = $derived(history.some(h => h.resolved_at))
  let showActiveOnly = $state(false)
  let filteredHistory = $derived(showActiveOnly ? history.filter(h => !h.resolved_at) : history)
</script>

<Layout>
  <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Alerts</h1>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={3} />
    </div>
  {:else}
    <!-- Webhooks -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
      <div class="flex items-center justify-between mb-1">
        <h3 class="text-sm font-semibold text-text-primary">Webhooks</h3>
        <Button size="sm" variant="secondary" onclick={openWebhookCreate}>Add Webhook</Button>
      </div>
      <p class="text-xs text-text-muted mb-3">Destinations where alert notifications are sent. Supports Slack, Discord, Telegram, or any custom HTTP endpoint.</p>
      {#if webhooks.length === 0}
        <p class="text-sm text-text-muted">No webhooks configured. Add one to start receiving alert notifications.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Name</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Type</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">URL</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each webhooks as w}
                <tr class="hover:bg-surface-hover cursor-pointer" onclick={() => openWebhookEdit(w)}>
                  <td class="py-3 px-4 text-text-primary">{w.name}</td>
                  <td class="py-3 px-4"><Badge variant="info">{typeLabels[w.type] || w.type}</Badge></td>
                  <td class="py-3 px-4 max-w-48 truncate text-text-secondary font-mono text-xs">{w.url}</td>
                  <td class="py-3 px-4">
                    <Button variant="danger" size="sm" onclick={(e) => { e.stopPropagation(); deleteTarget = { type: 'webhook', id: w.id, label: w.name } }}>Delete</Button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Alert Rules -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
      <div class="flex items-center justify-between mb-1">
        <h3 class="text-sm font-semibold text-text-primary">Alert Rules</h3>
        <Button size="sm" variant="secondary" onclick={openRuleCreate}>Add Rule</Button>
      </div>
      <p class="text-xs text-text-muted mb-3">Rules define when to trigger alerts. When a metric crosses the threshold for the specified duration, a notification is sent to the chosen webhook.</p>
      {#if rules.length === 0}
        <p class="text-sm text-text-muted">No alert rules. Create a rule to monitor your apps and get notified when something goes wrong.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Name</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Duration</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Webhook</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Status</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each rules as r}
                <tr class="hover:bg-surface-hover cursor-pointer" onclick={() => openRuleEdit(r)}>
                  <td class="py-3 px-4 text-text-primary">{ruleName(r)}</td>
                  <td class="py-3 px-4 text-text-secondary">{formatDuration(r.duration_sec)}</td>
                  <td class="py-3 px-4 text-text-secondary">{webhookName(r.webhook_id)}</td>
                  <td class="py-3 px-4">
                    <Badge variant={r.enabled !== false ? 'success' : 'default'}>{r.enabled !== false ? 'Enabled' : 'Disabled'}</Badge>
                  </td>
                  <td class="py-3 px-4">
                    <Button variant="danger" size="sm" onclick={(e) => { e.stopPropagation(); deleteTarget = { type: 'rule', id: r.id, label: ruleName(r) } }}>Delete</Button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Alert History -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
      <div class="flex items-center justify-between mb-1">
        <h3 class="text-sm font-semibold text-text-primary">Alert History</h3>
        {#if history.length > 0}
          <div class="flex gap-2">
            {#if hasResolved}
              <Button size="sm" variant="ghost" onclick={() => deleteTarget = { type: 'history', mode: 'resolved', label: 'resolved alert history' }}>Clear Resolved</Button>
            {/if}
            <Button size="sm" variant="danger" onclick={() => deleteTarget = { type: 'history', mode: 'all', label: 'all alert history' }}>Clear All</Button>
          </div>
        {/if}
      </div>
      <div class="flex items-center justify-between mb-3">
        <p class="text-xs text-text-muted">Log of all triggered alerts and their resolution status.</p>
        {#if history.length > 0}
          <label class="flex items-center gap-1.5 text-xs text-text-muted cursor-pointer shrink-0">
            <input type="checkbox" bind:checked={showActiveOnly} class="rounded border-border/50 accent-accent" />
            Active only
          </label>
        {/if}
      </div>
      {#if history.length === 0}
        <p class="text-sm text-text-muted">No alerts fired yet.</p>
      {:else if filteredHistory.length === 0}
        <p class="text-sm text-text-muted">No active alerts.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Rule</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Value</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Fired</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Resolved</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Status</th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each filteredHistory as h}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4 text-text-primary text-xs">{historyName(h)}</td>
                  <td class="py-3 px-4 font-mono text-xs">{formatMetricValue(historyMetric(h), h.value)}</td>
                  <td class="py-3 px-4" title={h.fired_at ? new Date(h.fired_at).toLocaleString() : ''}>{timeAgo(h.fired_at)}</td>
                  <td class="py-3 px-4" title={h.resolved_at ? new Date(h.resolved_at).toLocaleString() : ''}>{h.resolved_at ? timeAgo(h.resolved_at) : '-'}</td>
                  <td class="py-3 px-4">
                    <Badge variant={h.resolved_at ? 'success' : 'danger'}>{h.resolved_at ? 'Resolved' : 'Active'}</Badge>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Webhook Modal -->
  <FormModal open={showWebhookModal} title={editingWebhook ? 'Edit Webhook' : 'Create Webhook'} onclose={() => showWebhookModal = false}>
    <form onsubmit={(e) => { e.preventDefault(); saveWebhook() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Name</label>
        <input bind:value={whName} required class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" placeholder="My Webhook" />
      </div>
      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Type</label>
        <select bind:value={whType} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          {#each webhookTypes as t}
            <option value={t}>{typeLabels[t]}</option>
          {/each}
        </select>
      </div>
      {#if whType === 'telegram' && tgUseBuilder}
        <div class="flex flex-col gap-3 p-3 rounded-lg bg-surface-3/50 border border-border/30">
          <div class="flex items-center justify-between">
            <span class="text-xs font-medium text-text-muted">Telegram Setup</span>
            <button type="button" class="text-xs text-accent hover:underline" onclick={() => tgUseBuilder = false}>Enter URL manually</button>
          </div>
          <div>
            <label class="block text-xs font-medium text-text-muted mb-1.5">Bot Token</label>
            <input bind:value={tgBotToken} required placeholder="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 font-mono" />
            <p class="text-xs text-text-muted mt-1">Open Telegram, message <strong>@BotFather</strong>, send <code class="px-1 py-0.5 bg-surface-3 rounded text-text-secondary">/newbot</code>, and follow the prompts. Copy the token it gives you.</p>
          </div>
          <div>
            <label class="block text-xs font-medium text-text-muted mb-1.5">Chat ID</label>
            <input bind:value={tgChatId} required placeholder="-1001234567890" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 font-mono" />
            <p class="text-xs text-text-muted mt-1">Add the bot to your group/channel, then message <strong>@userinfobot</strong> in that chat to get the Chat ID. For personal chats, message <strong>@userinfobot</strong> directly.</p>
          </div>
          {#if tgBotToken && tgChatId}
            <div class="text-xs text-text-muted bg-surface-3 rounded px-2 py-1.5 font-mono break-all">
              {`https://api.telegram.org/bot${tgBotToken}/sendMessage?chat_id=${tgChatId}`}
            </div>
          {/if}
        </div>
      {:else}
        <div>
          <div class="flex items-center justify-between mb-1.5">
            <label class="block text-xs font-medium text-text-muted">URL</label>
            {#if whType === 'telegram'}
              <button type="button" class="text-xs text-accent hover:underline" onclick={() => tgUseBuilder = true}>Use guided setup</button>
            {/if}
          </div>
          <input bind:value={whUrl} required placeholder={urlPlaceholders[whType]} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
          <p class="text-xs text-text-muted mt-1">{urlHelp[whType]}</p>
        </div>
      {/if}

      <!-- Advanced -->
      <button type="button" class="flex items-center gap-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors" onclick={() => whAdvancedOpen = !whAdvancedOpen}>
        <svg class="w-3.5 h-3.5 transition-transform {whAdvancedOpen ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        Advanced Options
      </button>
      {#if whAdvancedOpen}
        <div class="flex flex-col gap-4 pl-3 border-l-2 border-border/30">
          <div>
            <label class="block text-xs font-medium text-text-muted mb-1.5">Template Override</label>
            <textarea bind:value={whTemplate} rows="3" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 font-mono" placeholder="Custom template..."></textarea>
            <p class="text-xs text-text-muted mt-1">Available variables: {'{{.AppName}}'}, {'{{.Metric}}'}, {'{{.Value}}'}, {'{{.Threshold}}'}, {'{{.Operator}}'}, {'{{.Status}}'}, {'{{.FiredAt}}'}</p>
          </div>
          <div>
            <label class="block text-xs font-medium text-text-muted mb-1.5">Custom Headers</label>
            <textarea bind:value={whHeaders} rows="2" class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 font-mono" placeholder={'{"Authorization": "Bearer xxx"}'}></textarea>
            <p class="text-xs text-text-muted mt-1">JSON object, e.g. {'{"Authorization": "Bearer xxx"}'}</p>
          </div>
        </div>
      {/if}

      <!-- Preview -->
      <div class="border-t border-border/30 pt-4 mt-1">
        <h4 class="text-xs font-medium text-text-muted mb-2">Preview</h4>
        {#if whType === 'slack'}
          <div class="rounded-lg bg-white dark:bg-[#f8f8f8] p-3 border-l-4 border-gray-400">
            <pre class="text-sm text-gray-800 whitespace-pre-wrap font-sans">{previewText}</pre>
          </div>
        {:else if whType === 'discord'}
          <div class="rounded-lg bg-surface-3 p-3 border-l-4 border-purple-500">
            <pre class="text-sm text-text-primary whitespace-pre-wrap font-sans">{previewText}</pre>
          </div>
        {:else if whType === 'telegram'}
          <div class="rounded-2xl bg-surface-hover p-3 max-w-xs border border-blue-500/20">
            <pre class="text-sm text-text-primary whitespace-pre-wrap font-sans">{previewText}</pre>
          </div>
        {:else}
          <div class="rounded-lg bg-surface-3 p-3">
            <pre class="text-xs text-text-primary whitespace-pre-wrap font-mono">{previewText}</pre>
          </div>
        {/if}
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-2 pt-2">
        <Button variant="secondary" size="sm" loading={whTesting} onclick={testWebhook} type="button">Send Test</Button>
        <Button size="sm" loading={whSaving} type="submit">{editingWebhook ? 'Save' : 'Create'}</Button>
      </div>
    </form>
  </FormModal>

  <!-- Rule Modal -->
  <FormModal open={showRuleModal} title={editingRule ? 'Edit Alert Rule' : 'Create Alert Rule'} onclose={() => showRuleModal = false}>
    <form onsubmit={(e) => { e.preventDefault(); saveRule() }} class="flex flex-col gap-5">
      <!-- Rule summary -->
      <div class="rounded-lg bg-surface-3/50 border border-border/30 px-4 py-3">
        <p class="text-xs text-text-muted mb-1">This rule will fire when:</p>
        <p class="text-sm text-text-primary font-medium">
          {metricLabels[rMetric] || rMetric} of
          <span class="text-accent">{rApp || 'all apps'}</span>
          is {rOp} {rThreshold}{rMetric === 'mem_bytes' ? ` ${thresholdUnit}` : rMetric.includes('pct') ? '%' : ''}
          for {humanDuration(+rDuration) ? humanDuration(+rDuration).slice(2) : `${rDuration}s`}
        </p>
      </div>

      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">App</label>
        <select bind:value={rApp} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          <option value="">All Apps</option>
          {#each apps as a}<option value={a.Slug || a.slug}>{a.Slug || a.slug}</option>{/each}
        </select>
        <p class="text-xs text-text-muted mt-1">Choose a specific app or monitor all apps at once.</p>
      </div>

      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Metric</label>
        <select bind:value={rMetric} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          {#each Object.entries(metricLabels) as [val, label]}
            <option value={val}>{label}</option>
          {/each}
        </select>
        <p class="text-xs text-text-muted mt-1">{metricHelp[rMetric]}</p>
      </div>

      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Condition</label>
        <div class="flex items-center gap-2">
          <span class="text-xs text-text-muted shrink-0">Alert when value is</span>
          <select bind:value={rOp} class="px-2 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary w-20">
            <option value=">">above</option>
            <option value=">=">at or above</option>
            <option value="<">below</option>
            <option value="<=">at or below</option>
          </select>
          {#if rMetric === 'mem_bytes'}
            <input type="number" bind:value={rThreshold} min="0" step="any" class="px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 w-24" />
            <select bind:value={thresholdUnit} class="px-2 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary w-20">
              <option>MB</option>
              <option>GB</option>
            </select>
          {:else}
            <input type="number" bind:value={rThreshold} min="0" max="100" step="any" class="px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 w-24" />
            <span class="text-xs text-text-muted">%</span>
          {/if}
        </div>
      </div>

      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Duration</label>
        <div class="flex flex-wrap gap-1.5 mb-2">
          {#each durationPresets as preset}
            <button type="button"
              class="px-2.5 py-1 text-xs rounded-md border transition-colors {+rDuration === preset.value ? 'bg-accent/15 border-accent/40 text-accent' : 'border-border/50 text-text-secondary hover:border-border hover:text-text-primary'}"
              onclick={() => rDuration = preset.value}
            >{preset.label}</button>
          {/each}
        </div>
        <div class="flex items-center gap-2">
          <input type="number" bind:value={rDuration} min="1" class="px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30 w-28" />
          <span class="text-xs text-text-muted">seconds</span>
          {#if +rDuration > 0}
            <span class="text-xs text-text-muted">({humanDuration(+rDuration) ? humanDuration(+rDuration).slice(2) : ''})</span>
          {/if}
        </div>
        <p class="text-xs text-text-muted mt-1">The condition must stay true for this long before an alert fires. Helps avoid noisy alerts from brief spikes.</p>
      </div>

      <div>
        <label class="block text-xs font-medium text-text-muted mb-1.5">Notify via</label>
        {#if webhooks.length === 0}
          <div class="rounded-lg border border-border/30 bg-surface-3/30 p-3 text-center">
            <p class="text-sm text-text-muted mb-2">No webhooks configured yet.</p>
            <Button size="sm" variant="secondary" onclick={() => { showRuleModal = false; openWebhookCreate() }}>Create Webhook First</Button>
          </div>
        {:else}
          <select bind:value={rWebhook} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
            <option value="">Select webhook</option>
            {#each webhooks as w}<option value={String(w.id)}>{w.name} ({typeLabels[w.type] || w.type})</option>{/each}
          </select>
          <p class="text-xs text-text-muted mt-1">Where to send the alert notification when this rule triggers.</p>
        {/if}
      </div>

      <div class="flex items-center justify-between pt-1">
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" id="rule-enabled" bind:checked={rEnabled} class="rounded border-border/50 accent-accent" />
          <span class="text-sm text-text-primary">Enabled</span>
        </label>
        <Button size="sm" loading={rSaving} type="submit">{editingRule ? 'Save' : 'Create'}</Button>
      </div>
    </form>
  </FormModal>

  <!-- Delete Confirmation -->
  {#if deleteTarget}
    <Modal
      title="Confirm Delete"
      message={`Delete ${deleteTarget.type} '${deleteTarget.label}'?`}
      onConfirm={confirmDelete}
      onCancel={() => deleteTarget = null}
    />
  {/if}
</Layout>
