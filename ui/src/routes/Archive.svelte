<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Modal from '../components/Modal.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let archived = $state([])
  let loading = $state(true)
  let expanded = $state({})
  let confirmSlug = $state(null)
  let purging = $state(false)

  onMount(load)

  async function load() {
    loading = true
    const res = await api.listArchived()
    archived = Array.isArray(res.data) ? res.data : []
    loading = false
  }

  function toggle(slug) {
    expanded[slug] = !expanded[slug]
  }

  function fmtAbsolute(ts) {
    if (!ts) return ''
    try { return new Date(ts).toLocaleString() } catch { return String(ts) }
  }

  function fmtRelative(ts) {
    if (!ts) return ''
    const t = new Date(ts).getTime()
    if (Number.isNaN(t)) return ''
    const diff = Date.now() - t
    const s = Math.max(0, Math.floor(diff / 1000))
    if (s < 60) return `${s}s ago`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m ago`
    const h = Math.floor(m / 60)
    if (h < 24) return `${h}h ago`
    const d = Math.floor(h / 24)
    return `${d}d ago`
  }

  function askPurge(slug) {
    confirmSlug = slug
  }

  async function doPurge() {
    if (!confirmSlug) return
    purging = true
    const slug = confirmSlug
    const res = await api.purgeApp(slug)
    purging = false
    confirmSlug = null
    if (!res.error) await load()
  }
</script>

<Layout>
  <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
    <h1 class="text-xl font-semibold tracking-tight text-text-primary">Archived apps</h1>
  </div>

  {#if loading}
    <div class="space-y-4"><Skeleton type="card" count={2} /></div>
  {:else if archived.length === 0}
    <div class="bg-surface-2 rounded-xl p-6 shadow-sm border border-border/50">
      <p class="text-sm text-text-muted">No archived apps.</p>
    </div>
  {:else}
    <div class="bg-surface-2 rounded-xl shadow-sm border border-border/50 overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-border/50">
            <th class="text-left text-xs font-medium text-text-muted py-3 px-4">App</th>
            <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Domain</th>
            <th class="text-left text-xs font-medium text-text-muted py-3 px-4">Archived</th>
            <th class="py-3 px-4"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/30">
          {#each archived as app (app.slug)}
            {@const t = app.tombstone}
            <tr class="hover:bg-surface-hover">
              <td class="py-3 px-4">
                <div class="font-medium text-text-primary">{app.display_name || app.slug}</div>
                <div class="text-xs text-text-muted">{app.slug}</div>
              </td>
              <td class="py-3 px-4 text-text-secondary">{app.domain || ''}</td>
              <td class="py-3 px-4 text-text-secondary" title={fmtAbsolute(app.archived_at)}>
                {fmtRelative(app.archived_at)}
              </td>
              <td class="py-3 px-4">
                <div class="flex items-center justify-end gap-2">
                  <Button size="sm" variant="secondary" onclick={() => toggle(app.slug)}>
                    {expanded[app.slug] ? 'Hide details' : 'Details'}
                  </Button>
                  <Button size="sm" variant="danger" onclick={() => askPurge(app.slug)}>Clean up</Button>
                </div>
              </td>
            </tr>
            {#if expanded[app.slug]}
              <tr class="bg-surface-1/40">
                <td colspan="4" class="py-4 px-4">
                  {#if !t}
                    <p class="text-xs text-text-muted">No tombstone snapshot available.</p>
                  {:else}
                    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs">
                      <div>
                        <div class="font-medium text-text-secondary mb-1">Alert rules ({(t.alert_rules || []).length})</div>
                        {#if (t.alert_rules || []).length === 0}
                          <div class="text-text-muted">None</div>
                        {:else}
                          <ul class="space-y-1 text-text-secondary">
                            {#each t.alert_rules as r}
                              <li>{r.name || r.kind || 'rule'}</li>
                            {/each}
                          </ul>
                        {/if}
                      </div>
                      <div>
                        <div class="font-medium text-text-secondary mb-1">Backup configs ({(t.backup_configs || []).length})</div>
                        {#if (t.backup_configs || []).length === 0}
                          <div class="text-text-muted">None</div>
                        {:else}
                          <ul class="space-y-1 text-text-secondary">
                            {#each t.backup_configs as c}
                              <li>{c.name || c.strategy || 'config'}</li>
                            {/each}
                          </ul>
                        {/if}
                      </div>
                      <div>
                        <div class="font-medium text-text-secondary mb-1">Access ({(t.access || []).length})</div>
                        {#if (t.access || []).length === 0}
                          <div class="text-text-muted">None</div>
                        {:else}
                          <ul class="space-y-1 text-text-secondary">
                            {#each t.access as a}
                              <li>{a.username || a.user_id || 'user'}{a.role ? ` (${a.role})` : ''}</li>
                            {/each}
                          </ul>
                        {/if}
                      </div>
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if confirmSlug}
    <Modal
      title="Permanently clean up {confirmSlug}?"
      message="This permanently deletes the app row and all history (audit, deploys, backups, alerts). This cannot be undone."
      onConfirm={doPurge}
      onCancel={() => (confirmSlug = null)}
    />
  {/if}
</Layout>
