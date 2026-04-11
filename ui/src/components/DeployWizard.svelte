<script>
  import Button from './Button.svelte'

  let { onclose = () => {}, onComplete = () => {} } = $props()

  let step = $state(1)

  const steps = ['Compose', 'Review', 'Deploy']
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
      <p class="text-text-muted text-sm">Step 1 placeholder</p>
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
    {#if step < 3}
      <Button size="sm" onclick={() => step++}>Next</Button>
    {/if}
  </div>
</div>
