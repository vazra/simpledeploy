<script>
  let { variant = 'default', children } = $props()

  const variants = {
    default: 'bg-surface-3 text-text-secondary',
    success: 'bg-green-900/30 text-success light:bg-green-100 light:text-green-800',
    danger: 'bg-red-900/30 text-danger light:bg-red-100 light:text-red-800',
    warning: 'bg-yellow-900/30 text-warning light:bg-yellow-100 light:text-yellow-800',
    info: 'bg-blue-900/30 text-accent light:bg-blue-100 light:text-blue-800',
  }
</script>

<span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {variants[variant]}">
  {@render children()}
</span>
