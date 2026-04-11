<script>
  import Button from './Button.svelte'

  let { onclose = () => {}, onComplete = () => {} } = $props()

  let step = $state(1)

  const steps = ['Compose', 'Review', 'Deploy']

  // Step 1 state
  let appName = $state('')
  let nameError = $state('')

  const NAME_REGEX = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$/

  function validateName(val) {
    if (!val.trim()) return 'App name is required'
    if (!NAME_REGEX.test(val)) return 'Must start with alphanumeric, then alphanumeric/dot/hyphen/underscore, max 63 chars'
    return ''
  }

  function handleNameInput(e) {
    appName = e.currentTarget.value
    nameError = appName.trim() ? validateName(appName) : ''
  }
</script>

<div class="flex flex-col h-full">
  <!-- Step indicator -->
  <div class="flex items-center gap-2 mb-6">
    {#each steps as label, i}
      {@const num = i + 1}
      {@const active = step === num}
      {@const done = step > num}
      <div class="flex items-center gap-2 {i > 0 ? 'flex-1' : ''}">
        {#if i > 0}
          <div class="flex-1 h-px {done ? 'bg-accent' : 'bg-border/50'}"></div>
        {/if}
        <div class="flex items-center gap-1.5">
          <div class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium transition-colors
            {active ? 'bg-accent text-white' : done ? 'bg-accent/20 text-accent' : 'bg-surface-3 text-text-muted'}">
            {#if done}
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
            {:else}
              {num}
            {/if}
          </div>
          <span class="text-xs font-medium {active ? 'text-text-primary' : 'text-text-muted'}">{label}</span>
        </div>
      </div>
    {/each}
  </div>

  <!-- Step content -->
  <div class="flex-1 overflow-y-auto">
    {#if step === 1}
      <div class="flex flex-col gap-4">
        <div>
          <label class="block text-xs font-medium text-text-muted mb-2">App Name</label>
          <input
            value={appName}
            oninput={handleNameInput}
            placeholder="my-app"
            class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
              {nameError ? 'border-danger/50' : 'border-border/50'}"
          />
          {#if nameError}
            <p class="text-xs text-danger mt-1">{nameError}</p>
          {:else}
            <p class="text-xs text-text-muted mt-1">Alphanumeric, dots, hyphens, underscores. Max 63 chars.</p>
          {/if}
        </div>

        <!-- Compose editor placeholder for next task -->
        <p class="text-text-muted text-sm">Compose editor coming next</p>
      </div>
    {:else if step === 2}
      <p class="text-text-muted text-sm">Step 2 placeholder</p>
    {:else}
      <p class="text-text-muted text-sm">Step 3 placeholder</p>
    {/if}
  </div>

  <!-- Footer -->
  <div class="flex justify-between pt-4 border-t border-border/50 mt-4">
    {#if step > 1 && step < 3}
      <Button variant="secondary" size="sm" onclick={() => step--}>Back</Button>
    {:else}
      <div></div>
    {/if}
    {#if step === 1}
      <Button size="sm" disabled={!appName.trim() || !!nameError} onclick={() => step++}>Next</Button>
    {:else if step < 3}
      <Button size="sm" onclick={() => step++}>Next</Button>
    {/if}
  </div>
</div>
