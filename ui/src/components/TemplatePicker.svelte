<script>
  import { generateSecret, validateVars } from '../lib/appTemplates.js'

  let {
    templates = [],
    categories = [],
    initialTemplateId = null,
    onapply = () => {},
    onblank = () => {},
  } = $props()

  // View state
  let view = $state('grid') // 'grid' | 'vars'
  let selected = $state(null)

  // Grid filters
  let search = $state('')
  let activeCategory = $state('all')

  // Vars form state
  let values = $state({})
  let errors = $state({})
  let revealed = $state({}) // keys to show secret in plain text
  let copiedKey = $state(null)

  // If initialTemplateId given, jump straight to vars form (only once)
  let jumped = $state(false)
  $effect(() => {
    if (!jumped && initialTemplateId && templates.length) {
      const t = templates.find((x) => x.id === initialTemplateId)
      if (t) {
        openTemplate(t)
        jumped = true
      }
    }
  })

  // Track copy "check" icon timer so we can clear on unmount / re-copy
  let copyTimer = null
  $effect(() => () => clearTimeout(copyTimer))

  function openTemplate(template) {
    selected = template
    values = {}
    errors = {}
    revealed = {}
    // Prefill defaults and auto-generate secrets
    for (const v of template.variables || []) {
      if (v.default != null) {
        values[v.key] = v.default
      }
      if (v.type === 'secret' && v.generate) {
        try {
          values[v.key] = generateSecret(v.generate.length, v.generate.charset)
        } catch (e) {
          // crypto unavailable; leave blank
        }
      }
    }
    view = 'vars'
  }

  function backToGrid() {
    view = 'grid'
    selected = null
    values = {}
    errors = {}
    revealed = {}
  }

  let filtered = $derived(
    templates.filter((t) => {
      if (activeCategory !== 'all' && t.category !== activeCategory) return false
      if (search.trim() === '') return true
      const hay = (t.name + ' ' + t.description + ' ' + (t.tags || []).join(' ')).toLowerCase()
      return hay.includes(search.toLowerCase())
    })
  )

  function categoryLabel(id) {
    const c = categories.find((x) => x.id === id)
    return c ? c.label : id
  }

  function handleBlur(v) {
    const errs = validateVars(selected.variables, values)
    if (errs[v.key]) {
      errors = { ...errors, [v.key]: errs[v.key] }
    } else {
      const next = { ...errors }
      delete next[v.key]
      errors = next
    }
  }

  function setValue(key, val) {
    values = { ...values, [key]: val }
  }

  function toggleReveal(key) {
    revealed = { ...revealed, [key]: !revealed[key] }
  }

  function regenerate(v) {
    try {
      const fresh = generateSecret(v.generate.length, v.generate.charset)
      setValue(v.key, fresh)
      const next = { ...errors }
      delete next[v.key]
      errors = next
    } catch (e) {
      // ignore
    }
  }

  async function copyValue(key) {
    try {
      await navigator.clipboard.writeText(String(values[key] ?? ''))
      copiedKey = key
      clearTimeout(copyTimer)
      copyTimer = setTimeout(() => {
        if (copiedKey === key) copiedKey = null
      }, 1200)
    } catch (e) {
      // ignore
    }
  }

  function handleApply() {
    if (!selected) return
    // Fill defaults for any missing values
    const merged = { ...values }
    for (const v of selected.variables || []) {
      if ((merged[v.key] == null || merged[v.key] === '') && v.default != null) {
        merged[v.key] = v.default
      }
    }
    const errs = validateVars(selected.variables, merged)
    errors = errs
    if (Object.keys(errs).length > 0) return
    values = merged
    onapply({ template: selected, vars: merged })
  }

  let hasErrors = $derived(Object.keys(errors).length > 0)

  // Split variables by hidden flag
  let primaryVars = $derived(
    selected ? (selected.variables || []).filter((v) => !v.hidden) : []
  )
  let hiddenVars = $derived(
    selected ? (selected.variables || []).filter((v) => v.hidden) : []
  )
</script>

{#if view === 'grid'}
  <div class="flex flex-col gap-4">
    <!-- Search + category chips -->
    <div class="flex flex-col gap-3">
      <div class="relative max-w-md">
        <span class="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted pointer-events-none">
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-4.35-4.35M10.5 18a7.5 7.5 0 1 1 0-15 7.5 7.5 0 0 1 0 15Z" />
          </svg>
        </span>
        <input
          type="text"
          bind:value={search}
          placeholder="Search templates..."
          class="w-full pl-9 pr-3 py-2 bg-input-bg border border-border/50 rounded-lg text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent/30"
        />
      </div>

      <div class="flex gap-1.5 overflow-x-auto pb-1 -mx-1 px-1">
        <button
          type="button"
          onclick={() => (activeCategory = 'all')}
          class="shrink-0 px-3 py-1 text-xs font-medium rounded-full transition-colors
            {activeCategory === 'all' ? 'bg-accent text-white' : 'bg-surface-3/40 text-text-muted hover:text-text-primary'}"
        >All</button>
        {#each categories as cat}
          <button
            type="button"
            onclick={() => (activeCategory = cat.id)}
            class="shrink-0 px-3 py-1 text-xs font-medium rounded-full transition-colors
              {activeCategory === cat.id ? 'bg-accent text-white' : 'bg-surface-3/40 text-text-muted hover:text-text-primary'}"
          >{cat.label}</button>
        {/each}
      </div>
    </div>

    <!-- Template cards -->
    {#if filtered.length === 0}
      <div class="text-center py-10 text-sm text-text-muted">No templates match.</div>
    {:else}
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
        {#each filtered as t}
          <button
            type="button"
            onclick={() => openTemplate(t)}
            class="relative text-left bg-surface-3/50 border border-border/30 rounded-lg px-4 py-3 cursor-pointer hover:border-accent/50 transition-colors min-h-[140px] flex flex-col"
            aria-label="Use template {t.name}"
          >
            {#if t.advanced}
              <span
                class="absolute top-2 right-2 text-[10px] font-medium bg-warning/15 text-warning px-1.5 py-0.5 rounded"
                title="Requires extra configuration after deploy."
              >Advanced</span>
            {/if}
            <div class="text-2xl mb-1.5" aria-hidden="true">{t.icon}</div>
            <div class="text-sm font-medium text-text-primary mb-0.5 pr-14">{t.name}</div>
            <p class="text-xs text-text-muted line-clamp-2 flex-1">{t.description}</p>
            <div class="mt-2">
              <span class="text-[10px] uppercase tracking-wider text-text-muted">{categoryLabel(t.category)}</span>
            </div>
          </button>
        {/each}
      </div>
    {/if}

    <!-- Start blank -->
    <button
      type="button"
      onclick={() => onblank()}
      class="w-full border border-dashed border-border/50 py-3 rounded-lg text-sm text-text-muted hover:text-text-primary hover:border-border transition-colors"
    >
      Start with a blank compose file &rarr;
    </button>
  </div>
{:else if view === 'vars' && selected}
  <div class="max-w-2xl mx-auto flex flex-col gap-5">
    <!-- Header -->
    <div class="flex flex-col gap-3">
      <button
        type="button"
        onclick={backToGrid}
        class="inline-flex items-center gap-1 text-xs text-text-muted hover:text-text-primary w-fit"
        aria-label="Back to templates"
      >
        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
        </svg>
        Back to templates
      </button>
      <div class="flex items-start gap-3">
        <div class="text-3xl" aria-hidden="true">{selected.icon}</div>
        <div class="flex-1">
          <h3 class="text-base font-semibold text-text-primary">{selected.name}</h3>
          <p class="text-xs text-text-muted mt-0.5">{selected.description}</p>
        </div>
      </div>
    </div>

    <!-- Primary variables -->
    {#if primaryVars.length > 0}
      <div class="flex flex-col gap-4">
        {#each primaryVars as v (v.key)}
          {@render fieldRow(v)}
        {/each}
      </div>
    {/if}

    <!-- Advanced / secrets -->
    {#if hiddenVars.length > 0}
      <details class="bg-surface-3/40 border border-border/30 rounded-lg">
        <summary class="cursor-pointer px-4 py-2.5 text-xs font-medium text-text-muted hover:text-text-primary select-none">
          Advanced / secrets (auto-generated)
        </summary>
        <div class="px-4 pb-4 pt-1 flex flex-col gap-4">
          {#each hiddenVars as v (v.key)}
            {@render fieldRow(v)}
          {/each}
        </div>
      </details>
    {/if}

    <!-- Footer -->
    <div class="flex justify-between pt-4 border-t border-border/50">
      <button
        type="button"
        onclick={backToGrid}
        class="px-4 py-2 text-sm text-text-muted hover:text-text-primary rounded-lg transition-colors"
      >Back</button>
      <button
        type="button"
        onclick={handleApply}
        disabled={hasErrors}
        class="px-4 py-2 text-sm font-medium bg-accent text-white rounded-lg hover:bg-accent/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >Apply &rarr;</button>
    </div>
  </div>
{/if}

{#snippet fieldRow(v)}
  <div>
    <label class="block text-sm font-medium text-text-primary mb-1.5" for="tpl-var-{v.key}">
      {v.label || v.key}
      {#if v.required}<span class="text-danger ml-0.5" aria-label="required">*</span>{/if}
    </label>

    {#if v.type === 'enum'}
      <select
        id="tpl-var-{v.key}"
        value={values[v.key] ?? ''}
        onchange={(e) => setValue(v.key, e.currentTarget.value)}
        onblur={() => handleBlur(v)}
        class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
          {errors[v.key] ? 'border-danger/50' : 'border-border/50'}"
      >
        <option value="" disabled>Select...</option>
        {#each v.options || [] as opt}
          <option value={opt.value}>{opt.label || opt.value}</option>
        {/each}
      </select>
    {:else if v.type === 'secret'}
      <div class="flex items-center gap-1.5">
        <input
          id="tpl-var-{v.key}"
          type={revealed[v.key] ? 'text' : 'password'}
          value={values[v.key] ?? ''}
          oninput={(e) => setValue(v.key, e.currentTarget.value)}
          onblur={() => handleBlur(v)}
          placeholder={v.placeholder || ''}
          class="flex-1 px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary font-mono focus:outline-none focus:ring-2 focus:ring-accent/30
            {errors[v.key] ? 'border-danger/50' : 'border-border/50'}"
        />
        <button
          type="button"
          onclick={() => toggleReveal(v.key)}
          class="p-2 text-text-muted hover:text-text-primary rounded-lg border border-border/50 hover:border-border transition-colors"
          aria-label={revealed[v.key] ? 'Hide' : 'Reveal'}
          title={revealed[v.key] ? 'Hide' : 'Reveal'}
        >
          {#if revealed[v.key]}
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3 3l18 18M10.58 10.58a2 2 0 0 0 2.83 2.83M9.88 5.08A10.94 10.94 0 0 1 12 5c5.52 0 10 4.48 10 7 0 1.1-.86 2.65-2.38 4.12M6.1 6.1C3.85 7.78 2 10.13 2 12c0 2.52 4.48 7 10 7 1.35 0 2.64-.27 3.82-.75" />
            </svg>
          {:else}
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
          {/if}
        </button>
        {#if v.generate}
          <button
            type="button"
            onclick={() => regenerate(v)}
            class="p-2 text-text-muted hover:text-text-primary rounded-lg border border-border/50 hover:border-border transition-colors"
            aria-label="Regenerate"
            title="Regenerate"
          >
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
            </svg>
          </button>
        {/if}
        <button
          type="button"
          onclick={() => copyValue(v.key)}
          class="p-2 text-text-muted hover:text-text-primary rounded-lg border border-border/50 hover:border-border transition-colors"
          aria-label="Copy"
          title={copiedKey === v.key ? 'Copied' : 'Copy'}
        >
          {#if copiedKey === v.key}
            <svg class="w-4 h-4 text-success" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          {:else}
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
            </svg>
          {/if}
        </button>
      </div>
    {:else if v.type === 'domain'}
      <input
        id="tpl-var-{v.key}"
        type="text"
        value={values[v.key] ?? ''}
        oninput={(e) => setValue(v.key, e.currentTarget.value)}
        onblur={() => handleBlur(v)}
        placeholder={v.placeholder || 'app.example.com'}
        pattern={v.pattern || '^[a-zA-Z0-9.-]+$'}
        class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
          {errors[v.key] ? 'border-danger/50' : 'border-border/50'}"
      />
    {:else if v.type === 'email'}
      <input
        id="tpl-var-{v.key}"
        type="email"
        value={values[v.key] ?? ''}
        oninput={(e) => setValue(v.key, e.currentTarget.value)}
        onblur={() => handleBlur(v)}
        placeholder={v.placeholder || ''}
        class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
          {errors[v.key] ? 'border-danger/50' : 'border-border/50'}"
      />
    {:else if v.type === 'number'}
      <input
        id="tpl-var-{v.key}"
        type="number"
        value={values[v.key] ?? ''}
        oninput={(e) => setValue(v.key, e.currentTarget.value)}
        onblur={() => handleBlur(v)}
        placeholder={v.placeholder || ''}
        class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
          {errors[v.key] ? 'border-danger/50' : 'border-border/50'}"
      />
    {:else}
      <input
        id="tpl-var-{v.key}"
        type="text"
        value={values[v.key] ?? ''}
        oninput={(e) => setValue(v.key, e.currentTarget.value)}
        onblur={() => handleBlur(v)}
        placeholder={v.placeholder || ''}
        pattern={v.pattern || undefined}
        class="w-full px-3 py-2 bg-input-bg border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-2 focus:ring-accent/30
          {errors[v.key] ? 'border-danger/50' : 'border-border/50'}"
      />
    {/if}

    {#if v.type === 'domain'}
      <p class="text-xs text-text-muted mt-1">Point this domain at your server's IP before continuing. TLS is provisioned automatically.</p>
    {:else if v.help}
      <p class="text-xs text-text-muted mt-1">{v.help}</p>
    {/if}

    {#if errors[v.key]}
      <p class="text-xs text-danger mt-1">{errors[v.key]}</p>
    {/if}
  </div>
{/snippet}
