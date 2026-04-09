<script>
  import { themePreference, effectiveTheme } from '../lib/stores/theme.js'

  const modes = ['system', 'light', 'dark']

  function cycle() {
    themePreference.update((current) => {
      const idx = modes.indexOf(current)
      return modes[(idx + 1) % modes.length]
    })
  }
</script>

<button
  onclick={cycle}
  class="relative flex items-center justify-center w-8 h-8 rounded-md text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors"
  title="Theme: {$themePreference}"
  aria-label="Toggle theme"
>
  {#if $effectiveTheme === 'dark'}
    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 006.963-2.998z" />
    </svg>
  {:else if $effectiveTheme === 'light'}
    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
    </svg>
  {/if}
  {#if $themePreference === 'system'}
    <span class="absolute -top-1 -right-1 w-2 h-2 bg-accent rounded-full"></span>
  {/if}
</button>
