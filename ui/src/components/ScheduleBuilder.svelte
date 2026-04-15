<script>
  let { value = '0 2 * * *', onchange } = $props()

  const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
  const DAY_FULL  = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

  // --- parse initial cron on mount ---
  function parseCron(expr) {
    const parts = (expr || '').trim().split(/\s+/)
    if (parts.length !== 5) return { freq: 'custom', hour: 2, minute: 0, weekdays: [], monthDay: 1, custom: expr }

    const [min, hr, dom, mon, dow] = parts

    // daily: minute hour * * *
    if (dom === '*' && mon === '*' && dow === '*' && /^\d+$/.test(hr) && /^\d+$/.test(min)) {
      return { freq: 'daily', hour: Number(hr), minute: Number(min), weekdays: [], monthDay: 1, custom: expr }
    }

    // weekly: minute hour * * 0,1,2...
    if (dom === '*' && mon === '*' && dow !== '*' && /^\d+$/.test(hr) && /^\d+$/.test(min)) {
      const days = dow.split(',').map(Number).filter(n => n >= 0 && n <= 6)
      return { freq: 'weekly', hour: Number(hr), minute: Number(min), weekdays: days, monthDay: 1, custom: expr }
    }

    // monthly: minute hour 1-28 * *
    if (mon === '*' && dow === '*' && /^\d+$/.test(dom) && /^\d+$/.test(hr) && /^\d+$/.test(min)) {
      return { freq: 'monthly', hour: Number(hr), minute: Number(min), weekdays: [], monthDay: Number(dom), custom: expr }
    }

    return { freq: 'custom', hour: 2, minute: 0, weekdays: [], monthDay: 1, custom: expr }
  }

  const parsed = parseCron(value)

  let frequency  = $state(parsed.freq)
  let hour       = $state(parsed.hour)
  let minute     = $state(parsed.minute)
  let weekdays   = $state(parsed.weekdays)
  let monthDay   = $state(parsed.monthDay)
  let customCron = $state(parsed.custom)

  // --- build cron from state ---
  function buildCron() {
    const hh = String(hour).padStart(2, '0')
    const mm = String(minute).padStart(2, '0')
    if (frequency === 'daily')   return `${minute} ${hour} * * *`
    if (frequency === 'weekly')  return weekdays.length ? `${minute} ${hour} * * ${weekdays.slice().sort().join(',')}` : `${minute} ${hour} * * *`
    if (frequency === 'monthly') return `${minute} ${hour} ${monthDay} * *`
    return customCron || '0 2 * * *'
  }

  // --- human-readable preview ---
  function buildPreview() {
    const pad = (n) => String(n).padStart(2, '0')
    const time = `${pad(hour)}:${pad(minute)}`

    if (frequency === 'daily') return `Every day at ${time}`

    if (frequency === 'weekly') {
      if (!weekdays.length) return `Every day at ${time}`
      const names = weekdays.slice().sort().map(d => DAY_NAMES[d]).join(', ')
      return `Every ${names} at ${time}`
    }

    if (frequency === 'monthly') return `On day ${monthDay} of every month at ${time}`

    return customCron || '—'
  }

  // emit changes whenever relevant state changes
  $effect(() => {
    // touch all reactive dependencies
    frequency; hour; minute; weekdays.length; weekdays.join(); monthDay; customCron
    const cron = buildCron()
    onchange?.(cron)
  })

  function toggleDay(idx) {
    if (weekdays.includes(idx)) {
      weekdays = weekdays.filter(d => d !== idx)
    } else {
      weekdays = [...weekdays, idx]
    }
  }

  // hours 0-23
  const HOURS = Array.from({ length: 24 }, (_, i) => i)
  // minutes at :00 :15 :30 :45
  const MINUTES = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]
  const MONTH_DAYS = Array.from({ length: 28 }, (_, i) => i + 1)

  const selectClass = 'bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/60 transition-colors'
  const labelClass  = 'text-xs text-text-muted'
</script>

<div class="space-y-4">

  <!-- Frequency selector -->
  <div>
    <div class="flex gap-0.5 bg-surface-3/40 rounded-lg p-0.5 w-fit">
      {#each ['daily', 'weekly', 'monthly', 'custom'] as freq}
        <button
          type="button"
          class="px-3 py-1.5 text-xs font-medium rounded-md transition-colors capitalize
            {frequency === freq
              ? 'bg-surface-2 text-text-primary shadow-sm'
              : 'text-text-muted hover:text-text-primary'}"
          onclick={() => frequency = freq}
        >{freq}</button>
      {/each}
    </div>
  </div>

  <!-- Daily -->
  {#if frequency === 'daily'}
    <div class="flex items-end gap-3">
      <div>
        <label class="{labelClass} block mb-1">Hour</label>
        <select class={selectClass} bind:value={hour}>
          {#each HOURS as h}
            <option value={h}>{String(h).padStart(2, '0')}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="{labelClass} block mb-1">Minute</label>
        <select class={selectClass} bind:value={minute}>
          {#each MINUTES as m}
            <option value={m}>{String(m).padStart(2, '0')}</option>
          {/each}
        </select>
      </div>
    </div>
  {/if}

  <!-- Weekly -->
  {#if frequency === 'weekly'}
    <div class="space-y-3">
      <div>
        <label class="{labelClass} block mb-1.5">Days of week</label>
        <div class="flex gap-1.5 flex-wrap">
          {#each DAY_NAMES as name, idx}
            <button
              type="button"
              class="w-10 h-8 rounded-md text-xs font-medium transition-colors
                {weekdays.includes(idx)
                  ? 'bg-accent text-white'
                  : 'bg-surface-3 text-text-muted hover:text-text-primary'}"
              onclick={() => toggleDay(idx)}
            >{name}</button>
          {/each}
        </div>
      </div>
      <div class="flex items-end gap-3">
        <div>
          <label class="{labelClass} block mb-1">Hour</label>
          <select class={selectClass} bind:value={hour}>
            {#each HOURS as h}
              <option value={h}>{String(h).padStart(2, '0')}</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="{labelClass} block mb-1">Minute</label>
          <select class={selectClass} bind:value={minute}>
            {#each MINUTES as m}
              <option value={m}>{String(m).padStart(2, '0')}</option>
            {/each}
          </select>
        </div>
      </div>
    </div>
  {/if}

  <!-- Monthly -->
  {#if frequency === 'monthly'}
    <div class="flex items-end gap-3">
      <div>
        <label class="{labelClass} block mb-1">Day of month</label>
        <select class={selectClass} bind:value={monthDay}>
          {#each MONTH_DAYS as d}
            <option value={d}>{d}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="{labelClass} block mb-1">Hour</label>
        <select class={selectClass} bind:value={hour}>
          {#each HOURS as h}
            <option value={h}>{String(h).padStart(2, '0')}</option>
          {/each}
        </select>
      </div>
      <div>
        <label class="{labelClass} block mb-1">Minute</label>
        <select class={selectClass} bind:value={minute}>
          {#each MINUTES as m}
            <option value={m}>{String(m).padStart(2, '0')}</option>
          {/each}
        </select>
      </div>
    </div>
  {/if}

  <!-- Custom -->
  {#if frequency === 'custom'}
    <div>
      <label class="{labelClass} block mb-1">Cron expression</label>
      <input
        type="text"
        class="bg-input-bg border border-border/50 rounded-lg px-3 py-2 text-sm text-text-primary font-mono w-full focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/60 transition-colors"
        placeholder="0 2 * * *"
        bind:value={customCron}
      />
      <p class="text-xs text-text-muted mt-1">Format: minute hour day-of-month month day-of-week</p>
    </div>
  {/if}

  <!-- Preview -->
  <div class="flex items-center gap-2 pt-1">
    <span class="text-xs text-text-secondary">{buildPreview()}</span>
    <span class="text-xs text-text-muted font-mono">({buildCron()})</span>
  </div>

</div>
