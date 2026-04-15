<script>
  let { app } = $props()

  function relativeTime(ts) {
    if (!ts) return 'Never'
    const now = Date.now()
    const then = new Date(ts).getTime()
    const diff = now - then
    const seconds = Math.floor(diff / 1000)
    const minutes = Math.floor(seconds / 60)
    const hours = Math.floor(minutes / 60)
    const days = Math.floor(hours / 24)

    if (seconds < 60) return 'just now'
    if (minutes < 60) return `${minutes}m ago`
    if (hours < 24) return `${hours}h ago`
    if (days < 365) return `${days}d ago`
    return 'Never'
  }

  function formatSize(bytes) {
    if (!bytes || bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    const value = (bytes / Math.pow(k, i)).toFixed(1)
    return `${value} ${sizes[i]}`
  }

  function strategyLabel(s) {
    if (s === 'postgres') return 'Database'
    if (s === 'volume') return 'Files'
    return s
  }

  const statusColor = app.last_run_status === 'success' ? 'bg-success' : app.last_run_status === 'failed' ? 'bg-danger' : 'bg-text-muted/50'
</script>

<a href="#/apps/{app.app_slug}?tab=backups" class="block bg-surface-2 rounded-xl p-4 shadow-sm border border-border/50 hover:shadow-lg hover:-translate-y-0.5 transition-all duration-200">
  <div class="flex items-start gap-3 mb-3">
    <span class="w-2 h-2 rounded-full shrink-0 ring-2 ring-surface-2 {statusColor}"></span>
    <h3 class="text-sm font-medium text-text-primary truncate">{app.app_name}</h3>
  </div>

  <div class="space-y-2">
    <div>
      <div class="text-xs text-text-muted mb-0.5">Last backup</div>
      <div class="text-xs text-text-secondary">{relativeTime(app.last_run_finished_at)}</div>
    </div>

    <div>
      <div class="text-xs text-text-muted mb-0.5">Configs</div>
      <div class="text-xs text-text-secondary">{app.config_count} · {app.strategies?.map(strategyLabel).join(', ') || 'None'}</div>
    </div>

    <div>
      <div class="text-xs text-text-muted mb-0.5">Storage used</div>
      <div class="text-xs text-text-secondary">{formatSize(app.total_size_bytes)}</div>
    </div>

    {#if app.recent_fail_count > 0}
      <div>
        <div class="text-xs text-text-muted mb-0.5">24h failures</div>
        <div class="text-xs text-danger font-medium">{app.recent_fail_count}</div>
      </div>
    {/if}
  </div>
</a>
