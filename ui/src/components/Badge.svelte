<script>
  let { variant = 'default', children } = $props()

  const variants = {
    default: 'bg-surface-3/60 text-text-secondary',
    success: 'bg-emerald-500/10 text-emerald-400 light:bg-emerald-50 light:text-emerald-700',
    danger: 'bg-red-500/10 text-red-400 light:bg-red-50 light:text-red-700',
    warning: 'bg-amber-500/10 text-amber-400 light:bg-amber-50 light:text-amber-700',
    info: 'bg-blue-500/10 text-blue-400 light:bg-blue-50 light:text-blue-700',
  }
</script>

<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium tracking-wide {variants[variant]}">
  {@render children()}
</span>
