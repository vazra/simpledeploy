<script>
  import Badge from './Badge.svelte'

  let { app, metrics = null } = $props()

  const statusVariant = {
    running: 'success',
    stopped: 'default',
    error: 'danger'
  }

  function formatBytes(bytes) {
    if (!bytes) return '0'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return (bytes / Math.pow(k, i)).toFixed(0) + ' ' + sizes[i]
  }
</script>

<a href="#/apps/{app.Slug}" class="block bg-surface-2 border border-border rounded-lg p-4 hover:border-accent hover:bg-surface-2/80 transition-all group">
  <div class="flex items-start justify-between mb-2">
    <div class="flex items-center gap-2 min-w-0">
      <span class="w-2 h-2 rounded-full shrink-0 {app.Status === 'running' ? 'bg-success' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
      <h3 class="text-sm font-semibold text-text-primary truncate group-hover:text-accent transition-colors">{app.Name}</h3>
    </div>
    <Badge variant={statusVariant[app.Status] || 'default'}>{app.Status}</Badge>
  </div>

  {#if app.Domain}
    <p class="text-xs text-accent truncate mb-3">{app.Domain}</p>
  {/if}

  {#if metrics}
    <div class="flex gap-3 pt-2 border-t border-border-muted">
      <div class="flex-1">
        <div class="text-xs text-text-muted mb-0.5">CPU</div>
        <div class="h-1.5 bg-surface-3 rounded-full overflow-hidden">
          <div class="h-full bg-accent rounded-full transition-all" style="width: {Math.min(metrics.cpu || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-text-secondary mt-0.5">{metrics.cpu?.toFixed(1) || 0}%</div>
      </div>
      <div class="flex-1">
        <div class="text-xs text-text-muted mb-0.5">MEM</div>
        <div class="h-1.5 bg-surface-3 rounded-full overflow-hidden">
          <div class="h-full bg-success rounded-full transition-all" style="width: {Math.min(metrics.memPct || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-text-secondary mt-0.5">{metrics.memPct?.toFixed(1) || 0}%</div>
      </div>
    </div>
  {/if}
</a>
