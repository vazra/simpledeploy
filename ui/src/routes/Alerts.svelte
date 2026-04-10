<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let webhooks = $state([])
  let rules = $state([])
  let history = $state([])
  let apps = $state([])
  let loading = $state(true)

  let showWebhookPanel = $state(false)
  let showRulePanel = $state(false)

  // webhook form
  let whName = $state('')
  let whType = $state('slack')
  let whUrl = $state('')

  // rule form
  let rApp = $state('')
  let rMetric = $state('cpu_pct')
  let rOp = $state('>')
  let rThreshold = $state(80)
  let rDuration = $state(60)
  let rWebhook = $state('')

  onMount(loadAll)

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

  async function createWebhook() {
    const res = await api.createWebhook({ name: whName, type: whType, url: whUrl })
    if (!res.error) { whName = ''; whUrl = ''; showWebhookPanel = false; loadAll() }
  }

  async function delWebhook(id) { await api.deleteWebhook(id); loadAll() }

  async function createRule() {
    const res = await api.createAlertRule({
      app_slug: rApp, metric: rMetric, operator: rOp,
      threshold: +rThreshold, duration_secs: +rDuration, webhook_id: +rWebhook,
    })
    if (!res.error) { showRulePanel = false; loadAll() }
  }

  async function delRule(id) { await api.deleteAlertRule(id); loadAll() }
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
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Webhooks</h3>
        <Button size="sm" variant="secondary" onclick={() => showWebhookPanel = true}>Add Webhook</Button>
      </div>
      {#if webhooks.length === 0}
        <p class="text-sm text-text-muted">No webhooks configured.</p>
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
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4">{w.name}</td>
                  <td class="py-3 px-4"><Badge>{w.type}</Badge></td>
                  <td class="py-3 px-4 max-w-48 truncate text-text-secondary">{w.url}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => delWebhook(w.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Alert Rules -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50 mb-6">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Alert Rules</h3>
        <Button size="sm" variant="secondary" onclick={() => showRulePanel = true}>Add Rule</Button>
      </div>
      {#if rules.length === 0}
        <p class="text-sm text-text-muted">No alert rules.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">App</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Metric</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Condition</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Duration</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Webhook</th>
              <th class="py-3 px-4"></th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each rules as r}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4">{r.app_slug || 'System'}</td>
                  <td class="py-3 px-4"><Badge variant="info">{r.metric}</Badge></td>
                  <td class="py-3 px-4 font-mono text-xs">{r.operator} {r.threshold}</td>
                  <td class="py-3 px-4">{r.duration_secs}s</td>
                  <td class="py-3 px-4">{r.webhook_id}</td>
                  <td class="py-3 px-4"><Button variant="danger" size="sm" onclick={() => delRule(r.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Alert History -->
    <div class="bg-surface-2 rounded-xl p-5 shadow-sm border border-border/50">
      <h3 class="text-sm font-semibold text-text-primary mb-3">Alert History</h3>
      {#if history.length === 0}
        <p class="text-sm text-text-muted">No alerts fired.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border/50">
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Rule</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Fired</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Resolved</th>
              <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Status</th>
            </tr></thead>
            <tbody class="divide-y divide-border/30">
              {#each history as h}
                <tr class="hover:bg-surface-hover">
                  <td class="py-3 px-4">#{h.rule_id}</td>
                  <td class="py-3 px-4">{new Date(h.fired_at).toLocaleString()}</td>
                  <td class="py-3 px-4">{h.resolved_at ? new Date(h.resolved_at).toLocaleString() : '-'}</td>
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

  <!-- Webhook Slide Panel -->
  <SlidePanel title="Add Webhook" open={showWebhookPanel} onclose={() => showWebhookPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createWebhook() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-muted mb-2">Name</label>
        <input bind:value={whName} required class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">Type</label>
        <select bind:value={whType} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          <option>slack</option><option>telegram</option><option>discord</option><option>custom</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">URL</label>
        <input bind:value={whUrl} required class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary focus:ring-2 focus:ring-accent/30" />
      </div>
      <Button type="submit">Create Webhook</Button>
    </form>
  </SlidePanel>

  <!-- Rule Slide Panel -->
  <SlidePanel title="Add Alert Rule" open={showRulePanel} onclose={() => showRulePanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createRule() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-muted mb-2">App</label>
        <select bind:value={rApp} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          <option value="">System-wide</option>
          {#each apps as a}<option value={a.Slug || a.slug}>{a.Slug || a.slug}</option>{/each}
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">Metric</label>
        <select bind:value={rMetric} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          <option>cpu_pct</option><option>mem_pct</option>
        </select>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-xs text-text-muted mb-2">Operator</label>
          <select bind:value={rOp} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
            <option value=">">&gt;</option><option value="<">&lt;</option><option value=">=">&gt;=</option><option value="<=">&lt;=</option>
          </select>
        </div>
        <div>
          <label class="block text-xs text-text-muted mb-2">Threshold</label>
          <input type="number" bind:value={rThreshold} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary" />
        </div>
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">Duration (seconds)</label>
        <input type="number" bind:value={rDuration} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary" />
      </div>
      <div>
        <label class="block text-xs text-text-muted mb-2">Webhook</label>
        <select bind:value={rWebhook} class="w-full px-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary">
          <option value="">Select webhook</option>
          {#each webhooks as w}<option value={w.id}>{w.name}</option>{/each}
        </select>
      </div>
      <Button type="submit">Create Rule</Button>
    </form>
  </SlidePanel>
</Layout>
