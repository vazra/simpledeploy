<script>
  let { variant = 'primary', size = 'md', loading = false, disabled = false, type = 'button', onclick, children } = $props()

  const base = 'inline-flex items-center justify-center font-medium rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-accent/50 disabled:opacity-50 disabled:cursor-not-allowed'

  const variants = {
    primary: 'bg-btn-primary hover:bg-btn-primary-hover text-white',
    secondary: 'bg-surface-3 hover:bg-surface-3/80 text-text-primary border border-border',
    danger: 'bg-btn-danger hover:bg-btn-danger/80 text-white',
    ghost: 'hover:bg-surface-3 text-text-secondary hover:text-text-primary',
  }

  const sizes = {
    sm: 'px-2.5 py-1 text-xs gap-1.5',
    md: 'px-3.5 py-2 text-sm gap-2',
    lg: 'px-5 py-2.5 text-base gap-2.5',
  }
</script>

<button
  {type}
  {onclick}
  disabled={disabled || loading}
  class="{base} {variants[variant]} {sizes[size]}"
>
  {#if loading}
    <svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
    </svg>
  {/if}
  {@render children()}
</button>
