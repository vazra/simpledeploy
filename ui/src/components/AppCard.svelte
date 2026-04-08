<script>
  let { app } = $props()

  const statusColors = {
    running: '#3fb950',
    stopped: '#8b949e',
    error: '#f85149'
  }
</script>

<a href="#/apps/{app.Slug}" class="card">
  <div class="header">
    <span class="status-dot" style="background: {statusColors[app.Status] || '#8b949e'}"></span>
    <h3>{app.Name}</h3>
  </div>
  <div class="meta">
    <span class="status">{app.Status}</span>
    {#if app.Domain}
      <span class="domain">{app.Domain}</span>
    {/if}
  </div>
</a>

<style>
  .card {
    display: block;
    background: #1c1f26;
    border: 1px solid #2d3139;
    border-radius: 8px;
    padding: 1rem;
    text-decoration: none;
    color: #e1e4e8;
    transition: border-color 0.15s, background 0.15s;
  }
  .card:hover {
    border-color: #58a6ff;
    background: #1f2430;
  }
  .header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }
  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  h3 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.78rem;
    color: #8b949e;
  }
  .status {
    text-transform: capitalize;
  }
  .domain {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
