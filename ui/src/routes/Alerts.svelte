<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import { api } from '../lib/api.js'

  let webhooks = $state([])
  let rules = $state([])
  let history = $state([])
  let apps = $state([])
  let error = $state('')

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
    try {
      error = ''
      ;[webhooks, rules, history, apps] = await Promise.all([
        api.listWebhooks().catch(() => []),
        api.listAlertRules().catch(() => []),
        api.alertHistory().catch(() => []),
        api.listApps().catch(() => []),
      ])
    } catch (e) { error = e.message }
  }

  async function createWebhook() {
    try { error = ''; await api.createWebhook({ name: whName, type: whType, url: whUrl }); whName = ''; whUrl = ''; await loadAll() }
    catch (e) { error = e.message }
  }

  async function delWebhook(id) {
    try { await api.deleteWebhook(id); await loadAll() } catch (e) { error = e.message }
  }

  async function createRule() {
    try {
      error = ''
      await api.createAlertRule({ app_slug: rApp, metric: rMetric, operator: rOp, threshold: +rThreshold, duration_secs: +rDuration, webhook_id: +rWebhook })
      await loadAll()
    } catch (e) { error = e.message }
  }

  async function delRule(id) {
    try { await api.deleteAlertRule(id); await loadAll() } catch (e) { error = e.message }
  }
</script>

<Layout>
  <h2 class="page-title">Alerts</h2>
  {#if error}<div class="error">{error}</div>{/if}

  <!-- Webhooks -->
  <div class="section">
    <h3 class="section-title">Webhooks</h3>
    {#if webhooks.length === 0}<p class="empty">No webhooks configured.</p>
    {:else}
      <table class="table">
        <thead><tr><th>Name</th><th>Type</th><th>URL</th><th></th></tr></thead>
        <tbody>
          {#each webhooks as w}
            <tr><td>{w.name}</td><td>{w.type}</td><td class="truncate">{w.url}</td>
              <td><button class="btn-danger-sm" onclick={() => delWebhook(w.id)}>Delete</button></td></tr>
          {/each}
        </tbody>
      </table>
    {/if}
    <form class="inline-form" onsubmit={(e) => { e.preventDefault(); createWebhook() }}>
      <input class="input" placeholder="Name" bind:value={whName} required />
      <select class="input sm" bind:value={whType}><option>slack</option><option>telegram</option><option>discord</option><option>custom</option></select>
      <input class="input" placeholder="URL" bind:value={whUrl} required />
      <button class="btn-primary" type="submit">Add</button>
    </form>
  </div>

  <!-- Alert Rules -->
  <div class="section">
    <h3 class="section-title">Alert Rules</h3>
    {#if rules.length === 0}<p class="empty">No alert rules.</p>
    {:else}
      <table class="table">
        <thead><tr><th>App</th><th>Metric</th><th>Condition</th><th>Duration</th><th>Webhook</th><th></th></tr></thead>
        <tbody>
          {#each rules as r}
            <tr><td>{r.app_slug}</td><td>{r.metric}</td><td>{r.operator} {r.threshold}</td><td>{r.duration_secs}s</td><td>{r.webhook_id}</td>
              <td><button class="btn-danger-sm" onclick={() => delRule(r.id)}>Delete</button></td></tr>
          {/each}
        </tbody>
      </table>
    {/if}
    <form class="inline-form" onsubmit={(e) => { e.preventDefault(); createRule() }}>
      <select class="input sm" bind:value={rApp}>
        <option value="">App</option>
        {#each apps as a}<option value={a.Slug || a.slug}>{a.Slug || a.slug}</option>{/each}
      </select>
      <select class="input sm" bind:value={rMetric}><option>cpu_pct</option><option>mem_pct</option></select>
      <select class="input xs" bind:value={rOp}><option value=">">&gt;</option><option value="<">&lt;</option><option value=">=">&gt;=</option><option value="<=">&lt;=</option></select>
      <input class="input xs" type="number" bind:value={rThreshold} placeholder="80" />
      <input class="input xs" type="number" bind:value={rDuration} placeholder="60" />
      <select class="input sm" bind:value={rWebhook}>
        <option value="">Webhook</option>
        {#each webhooks as w}<option value={w.id}>{w.name}</option>{/each}
      </select>
      <button class="btn-primary" type="submit">Add</button>
    </form>
  </div>

  <!-- Alert History -->
  <div class="section">
    <h3 class="section-title">Alert History</h3>
    {#if history.length === 0}<p class="empty">No alerts fired.</p>
    {:else}
      <table class="table">
        <thead><tr><th>Rule</th><th>Fired</th><th>Resolved</th></tr></thead>
        <tbody>
          {#each history as h}
            <tr><td>{h.rule_id}</td><td>{new Date(h.fired_at).toLocaleString()}</td><td>{h.resolved_at ? new Date(h.resolved_at).toLocaleString() : 'active'}</td></tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
</Layout>

<style>
  .page-title { font-size: 1.1rem; font-weight: 600; color: #e1e4e8; margin: 0 0 1rem; }
  .section { background: #1c1f26; border: 1px solid #2d3139; border-radius: 8px; padding: 1rem; margin-bottom: 1rem; }
  .section-title { font-size: 0.9rem; font-weight: 600; color: #e1e4e8; margin: 0 0 0.75rem; }
  .error { color: #f85149; font-size: 0.85rem; margin-bottom: 0.75rem; }
  .empty { color: #8b949e; font-size: 0.85rem; margin: 0 0 0.75rem; }
  .table { width: 100%; border-collapse: collapse; font-size: 0.82rem; margin-bottom: 0.75rem; }
  .table th { text-align: left; color: #8b949e; font-weight: 500; padding: 0.4rem 0.5rem; }
  .table td { padding: 0.4rem 0.5rem; color: #e1e4e8; }
  .table tbody tr:nth-child(even) { background: #161b22; }
  .truncate { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .inline-form { display: flex; gap: 0.4rem; align-items: center; flex-wrap: wrap; }
  .input { padding: 0.4rem 0.5rem; background: #0d1117; border: 1px solid #30363d; border-radius: 4px; color: #e1e4e8; font-size: 0.8rem; }
  .input.sm { width: 110px; }
  .input.xs { width: 70px; }
  .btn-primary { padding: 0.4rem 0.8rem; background: #238636; border: none; border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.8rem; white-space: nowrap; }
  .btn-primary:hover { background: #2ea043; }
  .btn-danger-sm { padding: 0.25rem 0.5rem; background: #da3633; border: none; border-radius: 4px; color: #fff; cursor: pointer; font-size: 0.75rem; }
</style>
