<script>
  let appName = $state('myapp');
  let image = $state('nginx:alpine');
  let domain = $state('myapp.example.com');
  let port = $state('80');
  let tls = $state('letsencrypt');
  let backup = $state('none');
  let backupSchedule = $state('0 3 * * *');
  let backupTarget = $state('local');
  let rateRequests = $state('');
  let rateWindow = $state('60s');
  let rateBy = $state('ip');
  let allowCidrs = $state('');
  let registryName = $state('');
  let copied = $state(false);

  function quote(v) { return `"${v}"`; }

  let yaml = $derived.by(() => {
    const lines = [];
    lines.push('services:');
    lines.push('  web:');
    lines.push(`    image: ${image}`);
    lines.push('    restart: unless-stopped');
    if (port) {
      lines.push('    ports:');
      lines.push(`      - "${port}:${port}"`);
    }
    const labels = [];
    if (domain) labels.push([`simpledeploy.endpoints.0.domain`, domain]);
    if (port) labels.push([`simpledeploy.endpoints.0.port`, port]);
    if (tls && tls !== 'letsencrypt') labels.push([`simpledeploy.endpoints.0.tls`, tls]);
    if (registryName) labels.push([`simpledeploy.registries`, registryName]);
    if (backup !== 'none') {
      labels.push([`simpledeploy.backup.strategy`, backup]);
      labels.push([`simpledeploy.backup.schedule`, backupSchedule]);
      labels.push([`simpledeploy.backup.target`, backupTarget]);
    }
    if (rateRequests) {
      labels.push([`simpledeploy.ratelimit.requests`, rateRequests]);
      labels.push([`simpledeploy.ratelimit.window`, rateWindow]);
      if (rateBy && rateBy !== 'ip') labels.push([`simpledeploy.ratelimit.by`, rateBy]);
    }
    if (allowCidrs.trim()) {
      labels.push([`simpledeploy.access.allow`, allowCidrs.trim()]);
    }
    if (labels.length) {
      lines.push('    labels:');
      for (const [k, v] of labels) lines.push(`      ${k}: ${quote(v)}`);
    }
    return lines.join('\n') + '\n';
  });

  let cliCmd = $derived(`simpledeploy apply -f docker-compose.yml --name ${appName || 'myapp'}`);

  async function copy() {
    try {
      await navigator.clipboard.writeText(yaml);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch (_) {
      copied = false;
    }
  }
</script>

<div class="playground">
  <div class="form">
    <fieldset>
      <legend>App</legend>
      <label>Name <input bind:value={appName} placeholder="myapp" /></label>
      <label>Image <input bind:value={image} placeholder="nginx:alpine" /></label>
      <label>Container port <input bind:value={port} placeholder="80" /></label>
    </fieldset>

    <fieldset>
      <legend>Routing & TLS</legend>
      <label>Domain <input bind:value={domain} placeholder="myapp.example.com" /></label>
      <label>TLS mode
        <select bind:value={tls}>
          <option value="letsencrypt">letsencrypt (default)</option>
          <option value="off">off (HTTP only)</option>
          <option value="custom">custom (upload cert)</option>
        </select>
      </label>
    </fieldset>

    <fieldset>
      <legend>Rate limit (optional)</legend>
      <label>Requests <input bind:value={rateRequests} placeholder="100" /></label>
      <label>Window <input bind:value={rateWindow} placeholder="60s" /></label>
      <label>Key by
        <select bind:value={rateBy}>
          <option value="ip">ip (default)</option>
          <option value="path">path</option>
          <option value="header:Authorization">header:Authorization</option>
        </select>
      </label>
    </fieldset>

    <fieldset>
      <legend>Access control (optional)</legend>
      <label>Allow CIDRs
        <input bind:value={allowCidrs} placeholder="10.0.0.0/8,192.168.1.5" />
      </label>
    </fieldset>

    <fieldset>
      <legend>Private registry (optional)</legend>
      <label>Registry name
        <input bind:value={registryName} placeholder="ghcr" />
      </label>
    </fieldset>

    <fieldset>
      <legend>Backup (optional)</legend>
      <label>Strategy
        <select bind:value={backup}>
          <option value="none">none</option>
          <option value="postgres">postgres</option>
          <option value="mysql">mysql</option>
          <option value="mongo">mongo</option>
          <option value="redis">redis</option>
          <option value="sqlite">sqlite</option>
          <option value="volume">volume</option>
        </select>
      </label>
      {#if backup !== 'none'}
        <label>Schedule (cron)
          <input bind:value={backupSchedule} placeholder="0 3 * * *" />
        </label>
        <label>Target
          <select bind:value={backupTarget}>
            <option value="local">local</option>
            <option value="s3://my-bucket/backups">s3://my-bucket/backups</option>
          </select>
        </label>
      {/if}
    </fieldset>
  </div>

  <div class="output">
    <div class="output-header">
      <strong>docker-compose.yml</strong>
      <button onclick={copy}>{copied ? 'Copied' : 'Copy'}</button>
    </div>
    <pre><code>{yaml}</code></pre>
    <p class="hint">Then run:</p>
    <pre><code>{cliCmd}</code></pre>
  </div>
</div>

<style>
  .playground {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1.5rem;
    margin: 1.5rem 0;
  }
  @media (max-width: 900px) {
    .playground { grid-template-columns: 1fr; }
  }
  fieldset {
    border: 1px solid var(--sl-color-gray-5);
    border-radius: 0.5rem;
    padding: 0.75rem 1rem 1rem;
    margin: 0 0 1rem 0;
  }
  legend {
    padding: 0 0.5rem;
    font-weight: 600;
    color: var(--sl-color-text-accent);
  }
  label {
    display: block;
    margin: 0.5rem 0;
    font-size: 0.9rem;
  }
  input, select {
    display: block;
    width: 100%;
    padding: 0.4rem 0.5rem;
    margin-top: 0.25rem;
    border: 1px solid var(--sl-color-gray-5);
    border-radius: 0.25rem;
    background: var(--sl-color-bg);
    color: var(--sl-color-text);
    font-family: inherit;
    font-size: 0.9rem;
  }
  .output {
    position: sticky;
    top: 5rem;
    align-self: start;
  }
  .output-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }
  button {
    background: var(--sl-color-accent);
    color: var(--sl-color-text-invert);
    border: 0;
    border-radius: 0.25rem;
    padding: 0.4rem 0.9rem;
    font-size: 0.85rem;
    cursor: pointer;
  }
  pre {
    background: var(--sl-color-gray-7);
    border-radius: 0.5rem;
    padding: 1rem;
    overflow-x: auto;
    font-size: 0.85rem;
  }
  .hint {
    margin: 0.75rem 0 0.25rem;
    font-size: 0.85rem;
    color: var(--sl-color-gray-3);
  }
</style>
