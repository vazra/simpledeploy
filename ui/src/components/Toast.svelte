<script>
  import { toasts } from '../lib/stores/toast.js'

  const typeStyles = {
    success: 'bg-green-900/80 border-success text-success',
    error: 'bg-red-900/80 border-danger text-danger',
    warning: 'bg-yellow-900/80 border-warning text-warning',
    info: 'bg-blue-900/80 border-accent text-accent',
  }

  const lightTypeStyles = {
    success: 'light:bg-green-50 light:border-success light:text-green-800',
    error: 'light:bg-red-50 light:border-danger light:text-red-800',
    warning: 'light:bg-yellow-50 light:border-warning light:text-yellow-800',
    info: 'light:bg-blue-50 light:border-accent light:text-blue-800',
  }

  const icons = {
    success: 'M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    error: 'M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z',
    warning: 'M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126z',
    info: 'M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z',
  }
</script>

<div class="fixed top-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
  {#each $toasts as toast (toast.id)}
    <div
      class="flex items-start gap-2 px-4 py-3 rounded-lg border text-sm shadow-lg backdrop-blur-sm animate-slide-in {typeStyles[toast.type]} {lightTypeStyles[toast.type]}"
      role="alert"
    >
      <svg class="w-5 h-5 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d={icons[toast.type]} />
      </svg>
      <span class="flex-1">{toast.message}</span>
      <button
        onclick={() => toasts.remove(toast.id)}
        class="shrink-0 opacity-60 hover:opacity-100 transition-opacity"
        aria-label="Dismiss"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  {/each}
</div>
