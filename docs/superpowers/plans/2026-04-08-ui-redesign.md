# UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the SimpleDeploy Svelte dashboard with Tailwind CSS, collapsible sidebar, dark/light theming, toast notifications, and information-rich dashboard.

**Architecture:** Replace all custom CSS with Tailwind CSS v4. Extract reusable components (Badge, Button, StatCard, etc.). Add theme/toast/sidebar stores. Rebuild every page with Tailwind utility classes.

**Tech Stack:** Svelte 5, Tailwind CSS v4 (Vite plugin), Chart.js, svelte-spa-router

**Spec:** `docs/superpowers/specs/2026-04-08-ui-redesign-design.md`

---

### Task 1: Tailwind CSS + Theme Foundation

**Files:**
- Modify: `ui/package.json`
- Modify: `ui/vite.config.js`
- Create: `ui/src/app.css`
- Modify: `ui/index.html`
- Modify: `ui/src/main.js`

- [ ] **Step 1: Install Tailwind CSS v4**

```bash
cd ui && npm install tailwindcss @tailwindcss/vite
```

- [ ] **Step 2: Configure Vite for Tailwind**

Modify `ui/vite.config.js` to:

```js
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8443',
      '/ws': { target: 'ws://localhost:8443', ws: true }
    }
  }
})
```

- [ ] **Step 3: Create Tailwind entry CSS with theme tokens**

Create `ui/src/app.css`:

```css
@import "tailwindcss";

@theme {
  --color-surface-0: #0f1117;
  --color-surface-1: #161b22;
  --color-surface-2: #1c1f26;
  --color-surface-3: #21262d;
  --color-border: #2d3139;
  --color-border-muted: #21262d;
  --color-text-primary: #e1e4e8;
  --color-text-secondary: #8b949e;
  --color-text-muted: #484f58;
  --color-accent: #58a6ff;
  --color-success: #3fb950;
  --color-danger: #f85149;
  --color-warning: #d29922;
  --color-info: #58a6ff;
  --color-input-bg: #0d1117;
  --color-btn-primary: #238636;
  --color-btn-primary-hover: #2ea043;
  --color-btn-danger: #da3633;
}

@custom-variant light, .light &;

@theme light {
  --color-surface-0: #ffffff;
  --color-surface-1: #f6f8fa;
  --color-surface-2: #ffffff;
  --color-surface-3: #d1d5db;
  --color-border: #d0d7de;
  --color-border-muted: #e5e7eb;
  --color-text-primary: #1f2937;
  --color-text-secondary: #656d76;
  --color-text-muted: #9ca3af;
  --color-accent: #0969da;
  --color-success: #1a7f37;
  --color-danger: #cf222e;
  --color-warning: #9a6700;
  --color-info: #0969da;
  --color-input-bg: #f6f8fa;
  --color-btn-primary: #1f883d;
  --color-btn-primary-hover: #1a7f37;
  --color-btn-danger: #cf222e;
}
```

- [ ] **Step 4: Update index.html**

Replace `ui/index.html` with:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>SimpleDeploy</title>
</head>
<body class="bg-surface-0 text-text-primary font-sans antialiased">
  <div id="app"></div>
  <script type="module" src="/src/main.js"></script>
</body>
</html>
```

- [ ] **Step 5: Import CSS in main.js**

Modify `ui/src/main.js` to:

```js
import './app.css'
import { mount } from 'svelte'
import App from './App.svelte'

const app = mount(App, { target: document.getElementById('app') })
export default app
```

- [ ] **Step 6: Verify build works**

```bash
cd ui && npm run build
```

Expected: Build succeeds with no errors.

- [ ] **Step 7: Commit**

```bash
git add ui/package.json ui/package-lock.json ui/vite.config.js ui/src/app.css ui/index.html ui/src/main.js
git commit -m "feat(ui): add tailwind css v4 with dark/light theme tokens"
```

---

### Task 2: Theme Store + ThemeToggle Component

**Files:**
- Create: `ui/src/lib/stores/theme.js`
- Create: `ui/src/components/ThemeToggle.svelte`

- [ ] **Step 1: Create theme store**

Create `ui/src/lib/stores/theme.js`:

```js
import { writable } from 'svelte/store'

const STORAGE_KEY = 'simpledeploy-theme'

function getInitialTheme() {
  if (typeof window === 'undefined') return 'system'
  return localStorage.getItem(STORAGE_KEY) || 'system'
}

function getEffectiveTheme(preference) {
  if (preference !== 'system') return preference
  if (typeof window === 'undefined') return 'dark'
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

function applyTheme(effective) {
  const root = document.documentElement
  if (effective === 'light') {
    root.classList.add('light')
  } else {
    root.classList.remove('light')
  }
}

const preference = getInitialTheme()
export const themePreference = writable(preference)
export const effectiveTheme = writable(getEffectiveTheme(preference))

themePreference.subscribe((pref) => {
  if (typeof window === 'undefined') return
  localStorage.setItem(STORAGE_KEY, pref)
  const effective = getEffectiveTheme(pref)
  effectiveTheme.set(effective)
  applyTheme(effective)
})

if (typeof window !== 'undefined') {
  window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', () => {
    let current
    themePreference.subscribe((v) => (current = v))()
    if (current === 'system') {
      const effective = getEffectiveTheme('system')
      effectiveTheme.set(effective)
      applyTheme(effective)
    }
  })
  applyTheme(getEffectiveTheme(preference))
}
```

- [ ] **Step 2: Create ThemeToggle component**

Create `ui/src/components/ThemeToggle.svelte`:

```svelte
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
  class="flex items-center justify-center w-8 h-8 rounded-md text-text-secondary hover:text-text-primary hover:bg-surface-3 transition-colors"
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
```

- [ ] **Step 3: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 4: Commit**

```bash
git add ui/src/lib/stores/theme.js ui/src/components/ThemeToggle.svelte
git commit -m "feat(ui): add theme store and toggle component"
```

---

### Task 3: Toast Store + Toast Component

**Files:**
- Create: `ui/src/lib/stores/toast.js`
- Create: `ui/src/components/Toast.svelte`

- [ ] **Step 1: Create toast store**

Create `ui/src/lib/stores/toast.js`:

```js
import { writable } from 'svelte/store'

let nextId = 0

function createToastStore() {
  const { subscribe, update } = writable([])

  function add(type, message, timeout = 4000) {
    const id = nextId++
    update((toasts) => [...toasts, { id, type, message }])
    if (timeout > 0) {
      setTimeout(() => remove(id), timeout)
    }
    return id
  }

  function remove(id) {
    update((toasts) => toasts.filter((t) => t.id !== id))
  }

  return {
    subscribe,
    success: (msg) => add('success', msg),
    error: (msg) => add('error', msg),
    warning: (msg) => add('warning', msg),
    info: (msg) => add('info', msg),
    remove,
  }
}

export const toasts = createToastStore()
```

- [ ] **Step 2: Create Toast component**

Create `ui/src/components/Toast.svelte`:

```svelte
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
```

- [ ] **Step 3: Add slide-in animation to app.css**

Append to `ui/src/app.css`:

```css
@keyframes slide-in {
  from { opacity: 0; transform: translateX(100%); }
  to { opacity: 1; transform: translateX(0); }
}

.animate-slide-in {
  animation: slide-in 0.2s ease-out;
}
```

- [ ] **Step 4: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 5: Commit**

```bash
git add ui/src/lib/stores/toast.js ui/src/components/Toast.svelte ui/src/app.css
git commit -m "feat(ui): add toast notification system"
```

---

### Task 4: Reusable Components (Badge, Button, StatCard, Skeleton, DataTable, SlidePanel, Modal)

**Files:**
- Create: `ui/src/components/Badge.svelte`
- Create: `ui/src/components/Button.svelte`
- Create: `ui/src/components/StatCard.svelte`
- Create: `ui/src/components/Skeleton.svelte`
- Create: `ui/src/components/DataTable.svelte`
- Create: `ui/src/components/SlidePanel.svelte`
- Modify: `ui/src/components/Modal.svelte`

- [ ] **Step 1: Create Badge component**

Create `ui/src/components/Badge.svelte`:

```svelte
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
```

- [ ] **Step 2: Create Button component**

Create `ui/src/components/Button.svelte`:

```svelte
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
```

- [ ] **Step 3: Create StatCard component**

Create `ui/src/components/StatCard.svelte`:

```svelte
<script>
  let { label, value, sub = '', icon = '', color = '' } = $props()
</script>

<div class="flex flex-col gap-1 bg-surface-2 border border-border rounded-lg p-4">
  <div class="flex items-center justify-between">
    <span class="text-xs font-medium text-text-secondary uppercase tracking-wider">{label}</span>
    {#if icon}
      <span class="text-text-muted">{@html icon}</span>
    {/if}
  </div>
  <span class="text-2xl font-semibold {color || 'text-text-primary'}">{value}</span>
  {#if sub}
    <span class="text-xs text-text-secondary">{sub}</span>
  {/if}
</div>
```

- [ ] **Step 4: Create Skeleton component**

Create `ui/src/components/Skeleton.svelte`:

```svelte
<script>
  let { type = 'card', count = 1 } = $props()
</script>

{#each Array(count) as _, i}
  {#if type === 'card'}
    <div class="bg-surface-2 border border-border rounded-lg p-4 animate-pulse">
      <div class="h-3 bg-surface-3 rounded w-1/3 mb-3"></div>
      <div class="h-6 bg-surface-3 rounded w-1/2 mb-2"></div>
      <div class="h-3 bg-surface-3 rounded w-2/3"></div>
    </div>
  {:else if type === 'table-row'}
    <div class="flex gap-4 py-3 animate-pulse">
      <div class="h-4 bg-surface-3 rounded w-1/4"></div>
      <div class="h-4 bg-surface-3 rounded w-1/3"></div>
      <div class="h-4 bg-surface-3 rounded w-1/6"></div>
    </div>
  {:else if type === 'chart'}
    <div class="bg-surface-2 border border-border rounded-lg p-4 animate-pulse">
      <div class="h-3 bg-surface-3 rounded w-1/4 mb-4"></div>
      <div class="h-44 bg-surface-3 rounded"></div>
    </div>
  {:else if type === 'line'}
    <div class="h-4 bg-surface-3 rounded w-full animate-pulse"></div>
  {/if}
{/each}
```

- [ ] **Step 5: Create DataTable component**

Create `ui/src/components/DataTable.svelte`:

```svelte
<script>
  let { columns = [], rows = [], emptyMessage = 'No data.' } = $props()
</script>

{#if rows.length === 0}
  <p class="text-text-secondary text-sm py-4">{emptyMessage}</p>
{:else}
  <div class="overflow-x-auto">
    <table class="w-full text-sm">
      <thead>
        <tr class="border-b border-border">
          {#each columns as col}
            <th class="text-left text-xs font-medium text-text-secondary uppercase tracking-wider py-2 px-3">{col.label}</th>
          {/each}
        </tr>
      </thead>
      <tbody class="divide-y divide-border-muted">
        {#each rows as row}
          <tr class="hover:bg-surface-1 transition-colors">
            {#each columns as col}
              <td class="py-2 px-3 text-text-primary">
                {#if col.render}
                  {@html col.render(row)}
                {:else}
                  {row[col.key] ?? ''}
                {/if}
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
```

- [ ] **Step 6: Create SlidePanel component**

Create `ui/src/components/SlidePanel.svelte`:

```svelte
<script>
  let { title = '', open = false, onclose = () => {}, children } = $props()
</script>

{#if open}
  <div class="fixed inset-0 z-40" role="dialog" aria-modal="true">
    <!-- backdrop -->
    <div class="absolute inset-0 bg-black/50" onclick={onclose}></div>
    <!-- panel -->
    <div class="absolute right-0 top-0 h-full w-full max-w-md bg-surface-2 border-l border-border shadow-xl flex flex-col animate-slide-panel">
      <div class="flex items-center justify-between px-5 py-4 border-b border-border">
        <h3 class="text-lg font-semibold text-text-primary">{title}</h3>
        <button onclick={onclose} class="text-text-secondary hover:text-text-primary" aria-label="Close panel">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <div class="flex-1 overflow-y-auto px-5 py-4">
        {@render children()}
      </div>
    </div>
  </div>
{/if}
```

- [ ] **Step 7: Rewrite Modal with Tailwind**

Replace `ui/src/components/Modal.svelte` with:

```svelte
<script>
  let { title = 'Confirm', message = '', onConfirm = () => {}, onCancel = () => {} } = $props()
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
  <div class="absolute inset-0 bg-black/60" onclick={onCancel}></div>
  <div class="relative bg-surface-2 border border-border rounded-lg p-6 min-w-80 max-w-md shadow-xl">
    <h3 class="text-base font-semibold text-text-primary mb-2">{title}</h3>
    <p class="text-sm text-text-secondary mb-5">{message}</p>
    <div class="flex justify-end gap-2">
      <button onclick={onCancel} class="px-3 py-1.5 text-sm border border-border rounded-md text-text-secondary hover:text-text-primary hover:border-text-secondary transition-colors">Cancel</button>
      <button onclick={onConfirm} class="px-3 py-1.5 text-sm bg-btn-danger text-white rounded-md hover:bg-btn-danger/80 transition-colors">Confirm</button>
    </div>
  </div>
</div>
```

- [ ] **Step 8: Add slide-panel animation to app.css**

Append to `ui/src/app.css`:

```css
@keyframes slide-panel {
  from { transform: translateX(100%); }
  to { transform: translateX(0); }
}

.animate-slide-panel {
  animation: slide-panel 0.2s ease-out;
}
```

- [ ] **Step 9: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 10: Commit**

```bash
git add ui/src/components/Badge.svelte ui/src/components/Button.svelte ui/src/components/StatCard.svelte ui/src/components/Skeleton.svelte ui/src/components/DataTable.svelte ui/src/components/SlidePanel.svelte ui/src/components/Modal.svelte ui/src/app.css
git commit -m "feat(ui): add reusable component library"
```

---

### Task 5: API Client Update

**Files:**
- Modify: `ui/src/lib/api.js`

- [ ] **Step 1: Rewrite API client with {data, error} pattern and toast integration**

Replace `ui/src/lib/api.js` with:

```js
import { toasts } from './stores/toast.js'

const BASE = '/api'

async function request(method, path, body = null) {
  const opts = {
    method,
    headers: {},
    credentials: 'include',
  }
  if (body) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  try {
    const res = await fetch(BASE + path, opts)
    if (res.status === 401) {
      if (!window.location.hash.includes('login')) {
        window.location.hash = '#/login'
      }
      return { data: null, error: 'Unauthorized' }
    }
    if (!res.ok) {
      const text = await res.text()
      const error = text || `HTTP ${res.status}`
      return { data: null, error }
    }
    const ct = res.headers.get('content-type')
    const data = ct && ct.includes('application/json') ? await res.json() : null
    return { data, error: null }
  } catch (err) {
    return { data: null, error: err.message }
  }
}

async function requestWithToast(method, path, body, successMsg) {
  const result = await request(method, path, body)
  if (result.error) {
    toasts.error(result.error)
  } else if (successMsg) {
    toasts.success(successMsg)
  }
  return result
}

export const api = {
  // Auth (no toast on success for login)
  login: (username, password) => request('POST', '/auth/login', { username, password }),
  logout: () => request('POST', '/auth/logout'),
  setup: (username, password) => request('POST', '/setup', { username, password }),
  health: () => request('GET', '/health'),

  // Apps
  listApps: () => request('GET', '/apps'),
  getApp: (slug) => request('GET', `/apps/${slug}`),
  removeApp: (slug) => requestWithToast('DELETE', `/apps/${slug}`, null, 'App removed'),

  // Metrics
  systemMetrics: (from, to) => request('GET', `/metrics/system?from=${from}&to=${to}`),
  appMetrics: (slug, from, to) => request('GET', `/apps/${slug}/metrics?from=${from}&to=${to}`),
  appRequests: (slug, from, to) => request('GET', `/apps/${slug}/requests?from=${from}&to=${to}`),

  // Backups
  listBackupConfigs: (slug) => request('GET', `/apps/${slug}/backups/configs`),
  createBackupConfig: (slug, cfg) => requestWithToast('POST', `/apps/${slug}/backups/configs`, cfg, 'Backup config created'),
  deleteBackupConfig: (id) => requestWithToast('DELETE', `/backups/configs/${id}`, null, 'Backup config deleted'),
  listBackupRuns: (slug) => request('GET', `/apps/${slug}/backups/runs`),
  triggerBackup: (slug) => requestWithToast('POST', `/apps/${slug}/backups/run`, null, 'Backup triggered'),
  restore: (id) => requestWithToast('POST', `/backups/restore/${id}`, null, 'Restore started'),

  // Webhooks
  listWebhooks: () => request('GET', '/webhooks'),
  createWebhook: (w) => requestWithToast('POST', '/webhooks', w, 'Webhook created'),
  deleteWebhook: (id) => requestWithToast('DELETE', `/webhooks/${id}`, null, 'Webhook deleted'),

  // Alerts
  listAlertRules: () => request('GET', '/alerts/rules'),
  createAlertRule: (r) => requestWithToast('POST', '/alerts/rules', r, 'Alert rule created'),
  deleteAlertRule: (id) => requestWithToast('DELETE', `/alerts/rules/${id}`, null, 'Alert rule deleted'),
  alertHistory: () => request('GET', '/alerts/history'),

  // Users
  listUsers: () => request('GET', '/users'),
  createUser: (u) => requestWithToast('POST', '/users', u, 'User created'),
  deleteUser: (id) => requestWithToast('DELETE', `/users/${id}`, null, 'User deleted'),
  listAPIKeys: () => request('GET', '/apikeys'),
  createAPIKey: (name) => requestWithToast('POST', '/apikeys', { name }, 'API key created'),
  deleteAPIKey: (id) => requestWithToast('DELETE', `/apikeys/${id}`, null, 'API key revoked'),
}
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/lib/api.js
git commit -m "feat(ui): update api client with error handling and toast integration"
```

---

### Task 6: Sidebar Component + Layout Rewrite

**Files:**
- Create: `ui/src/lib/stores/sidebar.js`
- Create: `ui/src/components/Sidebar.svelte`
- Modify: `ui/src/components/Layout.svelte`
- Modify: `ui/src/App.svelte`

- [ ] **Step 1: Create sidebar store**

Create `ui/src/lib/stores/sidebar.js`:

```js
import { writable } from 'svelte/store'

const STORAGE_KEY = 'simpledeploy-sidebar'

function getInitial() {
  if (typeof window === 'undefined') return true
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored !== null) return stored === 'true'
  return window.innerWidth >= 1024
}

export const sidebarExpanded = writable(getInitial())

sidebarExpanded.subscribe((val) => {
  if (typeof window !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, String(val))
  }
})
```

- [ ] **Step 2: Create Sidebar component**

Create `ui/src/components/Sidebar.svelte`:

```svelte
<script>
  import { sidebarExpanded } from '../lib/stores/sidebar.js'
  import ThemeToggle from './ThemeToggle.svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'

  let currentPath = $state(window.location.hash.slice(1) || '/')

  const nav = [
    { path: '/', label: 'Dashboard', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" /></svg>' },
    { path: '/alerts', label: 'Alerts', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" /></svg>' },
    { path: '/backups', label: 'Backups', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5m8.25 3v6.75m0 0l-3-3m3 3l3-3M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" /></svg>' },
    { path: '/users', label: 'Users', icon: '<svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" /></svg>' },
  ]

  function updatePath() {
    currentPath = window.location.hash.slice(1) || '/'
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('hashchange', updatePath)
  }

  function isActive(path) {
    if (path === '/') return currentPath === '/'
    return currentPath.startsWith(path)
  }

  async function logout() {
    await api.logout()
    push('/login')
  }

  function toggle() {
    sidebarExpanded.update((v) => !v)
  }
</script>

<aside class="flex flex-col h-screen bg-surface-1 border-r border-border transition-all duration-200 {$sidebarExpanded ? 'w-52' : 'w-14'}">
  <!-- Logo -->
  <div class="flex items-center h-14 px-3 border-b border-border">
    <div class="flex items-center gap-2 overflow-hidden">
      <svg class="w-7 h-7 shrink-0 text-accent" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
      </svg>
      {#if $sidebarExpanded}
        <span class="text-sm font-semibold text-accent whitespace-nowrap">SimpleDeploy</span>
      {/if}
    </div>
  </div>

  <!-- Nav -->
  <nav class="flex-1 flex flex-col gap-0.5 py-2 px-2">
    {#each nav as item}
      <a
        href="#{item.path}"
        class="flex items-center gap-2.5 px-2 py-2 rounded-md text-sm transition-colors
          {isActive(item.path) ? 'bg-surface-3 text-text-primary' : 'text-text-secondary hover:text-text-primary hover:bg-surface-3/50'}"
        title={$sidebarExpanded ? '' : item.label}
      >
        <span class="shrink-0">{@html item.icon}</span>
        {#if $sidebarExpanded}
          <span class="whitespace-nowrap">{item.label}</span>
        {/if}
      </a>
    {/each}
  </nav>

  <!-- Footer -->
  <div class="flex flex-col gap-1 p-2 border-t border-border">
    <div class="flex items-center {$sidebarExpanded ? 'justify-between' : 'justify-center'}">
      <ThemeToggle />
      {#if $sidebarExpanded}
        <button
          onclick={logout}
          class="text-xs text-text-secondary hover:text-danger transition-colors"
        >
          Logout
        </button>
      {/if}
    </div>
    <button
      onclick={toggle}
      class="flex items-center justify-center w-full py-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-surface-3/50 transition-colors"
      title={$sidebarExpanded ? 'Collapse sidebar' : 'Expand sidebar'}
      aria-label="Toggle sidebar"
    >
      <svg class="w-4 h-4 transition-transform {$sidebarExpanded ? '' : 'rotate-180'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
      </svg>
    </button>
  </div>
</aside>
```

- [ ] **Step 3: Rewrite Layout with Tailwind**

Replace `ui/src/components/Layout.svelte` with:

```svelte
<script>
  import Sidebar from './Sidebar.svelte'

  let { children } = $props()
</script>

<div class="flex min-h-screen bg-surface-0">
  <Sidebar />
  <main class="flex-1 overflow-y-auto p-6">
    {@render children()}
  </main>
</div>
```

- [ ] **Step 4: Update App.svelte to include Toast container**

Replace `ui/src/App.svelte` with:

```svelte
<script>
  import Router from 'svelte-spa-router'
  import Toast from './components/Toast.svelte'
  import Login from './routes/Login.svelte'
  import Dashboard from './routes/Dashboard.svelte'
  import AppDetail from './routes/AppDetail.svelte'
  import Backups from './routes/Backups.svelte'
  import Alerts from './routes/Alerts.svelte'
  import Users from './routes/Users.svelte'

  const routes = {
    '/login': Login,
    '/': Dashboard,
    '/apps/:slug': AppDetail,
    '/backups': Backups,
    '/alerts': Alerts,
    '/users': Users,
  }
</script>

<Router {routes} />
<Toast />
```

- [ ] **Step 5: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 6: Commit**

```bash
git add ui/src/lib/stores/sidebar.js ui/src/components/Sidebar.svelte ui/src/components/Layout.svelte ui/src/App.svelte
git commit -m "feat(ui): add collapsible sidebar and layout rewrite"
```

---

### Task 7: Login Page Rewrite

**Files:**
- Modify: `ui/src/routes/Login.svelte`

- [ ] **Step 1: Rewrite Login with Tailwind**

Replace `ui/src/routes/Login.svelte` with:

```svelte
<script>
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import Button from '../components/Button.svelte'

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let loading = $state(false)
  let setupMode = $state(false)

  async function handleSubmit(e) {
    e.preventDefault()
    error = ''
    loading = true
    try {
      if (setupMode) {
        const res = await api.setup(username, password)
        if (res.error) { error = res.error; loading = false; return }
      }
      const res = await api.login(username, password)
      if (res.error) { error = setupMode ? res.error : 'Invalid credentials'; loading = false; return }
      push('/')
    } catch (err) {
      error = err.message
    } finally {
      loading = false
    }
  }
</script>

<div class="flex items-center justify-center min-h-screen bg-surface-0 px-4">
  <div class="w-full max-w-sm">
    <div class="bg-surface-2 border border-border rounded-xl p-8 shadow-lg">
      <!-- Logo -->
      <div class="flex flex-col items-center mb-8">
        <svg class="w-10 h-10 text-accent mb-3" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
        </svg>
        <h1 class="text-xl font-bold text-accent">SimpleDeploy</h1>
        <p class="text-sm text-text-secondary mt-1">{setupMode ? 'Create Admin Account' : 'Sign in to continue'}</p>
      </div>

      {#if error}
        <div class="bg-red-900/20 border border-danger rounded-md px-3 py-2 mb-4 text-sm text-danger light:bg-red-50">
          {error}
        </div>
      {/if}

      <form onsubmit={handleSubmit} class="flex flex-col gap-4">
        <div>
          <label for="username" class="block text-xs font-medium text-text-secondary mb-1.5">Username</label>
          <input
            id="username"
            type="text"
            bind:value={username}
            required
            class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
          />
        </div>
        <div>
          <label for="password" class="block text-xs font-medium text-text-secondary mb-1.5">Password</label>
          <input
            id="password"
            type="password"
            bind:value={password}
            required
            class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-accent/50 focus:border-accent"
          />
        </div>
        <Button type="submit" {loading} variant="primary" size="md">
          {setupMode ? 'Create Account' : 'Sign In'}
        </Button>
      </form>

      <button
        onclick={() => { setupMode = !setupMode; error = '' }}
        class="block w-full mt-4 text-center text-xs text-accent hover:underline"
      >
        {setupMode ? 'Back to login' : 'First time? Create admin account'}
      </button>
    </div>
  </div>
</div>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/routes/Login.svelte
git commit -m "feat(ui): redesign login page with tailwind"
```

---

### Task 8: MetricsChart + AppCard Component Updates

**Files:**
- Modify: `ui/src/components/MetricsChart.svelte`
- Modify: `ui/src/components/AppCard.svelte`

- [ ] **Step 1: Rewrite MetricsChart with Tailwind and reactive data updates**

Replace `ui/src/components/MetricsChart.svelte` with:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'
  import { Chart, registerables } from 'chart.js'
  import 'chartjs-adapter-date-fns'
  import { effectiveTheme } from '../lib/stores/theme.js'

  Chart.register(...registerables)

  let { data = [], label = '', color = '#58a6ff', unit = '' } = $props()
  let canvas
  let chart

  function getGridColor(theme) {
    return theme === 'light' ? '#e5e7eb' : '#21262d'
  }

  function getTickColor(theme) {
    return theme === 'light' ? '#656d76' : '#8b949e'
  }

  function createChart(theme) {
    if (chart) chart.destroy()
    chart = new Chart(canvas, {
      type: 'line',
      data: {
        datasets: [{
          label,
          data,
          borderColor: color,
          backgroundColor: color + '20',
          fill: true,
          tension: 0.3,
          pointRadius: 0,
          borderWidth: 1.5,
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: {
            type: 'time',
            time: { unit: 'minute' },
            grid: { color: getGridColor(theme) },
            ticks: { color: getTickColor(theme), font: { size: 10 } }
          },
          y: {
            beginAtZero: true,
            grid: { color: getGridColor(theme) },
            ticks: {
              color: getTickColor(theme),
              font: { size: 10 },
              callback: (v) => v + unit
            }
          }
        },
        plugins: { legend: { display: false } }
      }
    })
  }

  onMount(() => {
    let currentTheme
    const unsub = effectiveTheme.subscribe((t) => {
      currentTheme = t
      if (canvas) createChart(t)
    })
    return unsub
  })

  $effect(() => {
    if (chart && data) {
      chart.data.datasets[0].data = data
      chart.update('none')
    }
  })

  onDestroy(() => { if (chart) chart.destroy() })
</script>

<div class="bg-surface-2 border border-border rounded-lg p-4">
  <h4 class="text-xs font-medium text-text-secondary mb-3">{label}</h4>
  <div class="h-44 relative">
    <canvas bind:this={canvas}></canvas>
  </div>
</div>
```

- [ ] **Step 2: Rewrite AppCard with Tailwind and richer info**

Replace `ui/src/components/AppCard.svelte` with:

```svelte
<script>
  import Badge from './Badge.svelte'

  let { app, metrics = null } = $props()

  const statusVariant = {
    running: 'success',
    stopped: 'default',
    error: 'danger'
  }

  function formatBytes(bytes) {
    if (!bytes) return '0'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return (bytes / Math.pow(k, i)).toFixed(0) + ' ' + sizes[i]
  }
</script>

<a href="#/apps/{app.Slug}" class="block bg-surface-2 border border-border rounded-lg p-4 hover:border-accent hover:bg-surface-2/80 transition-all group">
  <div class="flex items-start justify-between mb-2">
    <div class="flex items-center gap-2 min-w-0">
      <span class="w-2 h-2 rounded-full shrink-0 {app.Status === 'running' ? 'bg-success' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
      <h3 class="text-sm font-semibold text-text-primary truncate group-hover:text-accent transition-colors">{app.Name}</h3>
    </div>
    <Badge variant={statusVariant[app.Status] || 'default'}>{app.Status}</Badge>
  </div>

  {#if app.Domain}
    <p class="text-xs text-accent truncate mb-3">{app.Domain}</p>
  {/if}

  {#if metrics}
    <div class="flex gap-3 pt-2 border-t border-border-muted">
      <div class="flex-1">
        <div class="text-xs text-text-muted mb-0.5">CPU</div>
        <div class="h-1.5 bg-surface-3 rounded-full overflow-hidden">
          <div class="h-full bg-accent rounded-full transition-all" style="width: {Math.min(metrics.cpu || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-text-secondary mt-0.5">{metrics.cpu?.toFixed(1) || 0}%</div>
      </div>
      <div class="flex-1">
        <div class="text-xs text-text-muted mb-0.5">MEM</div>
        <div class="h-1.5 bg-surface-3 rounded-full overflow-hidden">
          <div class="h-full bg-success rounded-full transition-all" style="width: {Math.min(metrics.memPct || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-text-secondary mt-0.5">{metrics.memPct?.toFixed(1) || 0}%</div>
      </div>
    </div>
  {/if}
</a>
```

- [ ] **Step 3: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 4: Commit**

```bash
git add ui/src/components/MetricsChart.svelte ui/src/components/AppCard.svelte
git commit -m "feat(ui): rewrite MetricsChart and AppCard with tailwind"
```

---

### Task 9: LogViewer Rewrite

**Files:**
- Modify: `ui/src/components/LogViewer.svelte`

- [ ] **Step 1: Rewrite LogViewer with Tailwind**

Replace `ui/src/components/LogViewer.svelte` with:

```svelte
<script>
  import { onMount, onDestroy } from 'svelte'

  let { slug, service = '' } = $props()

  let lines = $state([])
  let ws = $state(null)
  let following = $state(true)
  let container
  let services = $state([])
  let selectedService = $state(service)
  let showTimestamps = $state(true)

  onMount(() => { connect() })
  onDestroy(() => { if (ws) ws.close() })

  function connect() {
    if (ws) ws.close()
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    let url = `${proto}//${window.location.host}/api/apps/${slug}/logs?follow=true&tail=200`
    if (selectedService) url += `&service=${selectedService}`

    ws = new WebSocket(url)
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      lines = [...lines.slice(-999), msg]
      if (following && container) {
        requestAnimationFrame(() => {
          container.scrollTop = container.scrollHeight
        })
      }
    }
    ws.onclose = () => { ws = null }
  }

  function toggleFollow() {
    following = !following
    if (following && container) {
      container.scrollTop = container.scrollHeight
    }
  }

  function clear() { lines = [] }

  function downloadLogs() {
    const text = lines.map((l) => `${l.ts || ''} [${l.stream}] ${l.line}`).join('\n')
    const blob = new Blob([text], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${slug}-logs.txt`
    a.click()
    URL.revokeObjectURL(url)
  }
</script>

<div class="flex flex-col h-[500px]">
  <!-- Toolbar -->
  <div class="flex items-center gap-2 px-3 py-2 bg-surface-1 border border-border rounded-t-lg">
    <button
      onclick={toggleFollow}
      class="px-2 py-1 text-xs rounded border transition-colors
        {following ? 'border-success text-success' : 'border-border text-text-secondary hover:text-text-primary'}"
    >
      {following ? 'Following' : 'Paused'}
    </button>
    <button onclick={clear} class="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors">
      Clear
    </button>
    <button
      onclick={() => showTimestamps = !showTimestamps}
      class="px-2 py-1 text-xs rounded border transition-colors
        {showTimestamps ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
    >
      Timestamps
    </button>
    <button onclick={downloadLogs} class="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:text-text-primary transition-colors">
      Download
    </button>
    <span class="ml-auto text-xs text-text-muted">{lines.length} lines</span>
  </div>

  <!-- Log output -->
  <div
    bind:this={container}
    class="flex-1 overflow-y-auto bg-surface-0 border border-t-0 border-border rounded-b-lg font-mono text-xs p-3 space-y-px"
  >
    {#each lines as line}
      <div class="whitespace-pre-wrap break-all {line.stream === 'stderr' ? 'text-danger' : 'text-text-primary'}">
        {#if showTimestamps && line.ts}<span class="text-text-muted mr-2">{line.ts}</span>{/if}<span>{line.line}</span>
      </div>
    {/each}
  </div>
</div>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/LogViewer.svelte
git commit -m "feat(ui): rewrite LogViewer with tailwind and download support"
```

---

### Task 10: Dashboard Redesign

**Files:**
- Modify: `ui/src/routes/Dashboard.svelte`

- [ ] **Step 1: Rewrite Dashboard with full info panels**

Replace `ui/src/routes/Dashboard.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import AppCard from '../components/AppCard.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import Badge from '../components/Badge.svelte'
  import { api } from '../lib/api.js'

  let apps = $state([])
  let cpuHistory = $state([])
  let memHistory = $state([])
  let loading = $state(true)
  let latestMetrics = $state(null)
  let alertRules = $state([])
  let alertHistory = $state([])
  let backupRunsByApp = $state({})
  let appMetricsMap = $state({})
  let appRequestsMap = $state({})
  let timeRange = $state('1h')

  const rangeMs = { '1h': 3600000, '6h': 21600000, '24h': 86400000, '7d': 604800000 }

  let filterStatus = $state('all')
  let sortBy = $state('name')

  onMount(loadDashboard)

  async function loadDashboard() {
    loading = true
    const now = new Date().toISOString()
    const from = new Date(Date.now() - rangeMs[timeRange]).toISOString()

    const [appsRes, metricsRes, rulesRes, histRes] = await Promise.all([
      api.listApps(),
      api.systemMetrics(from, now),
      api.listAlertRules(),
      api.alertHistory(),
    ])

    apps = appsRes.data || []
    alertRules = rulesRes.data || []
    alertHistory = histRes.data || []

    const metricsData = metricsRes.data || []
    if (metricsData.length > 0) {
      const latest = metricsData[metricsData.length - 1]
      latestMetrics = {
        cpu: latest.cpu_pct?.toFixed(1) || '0',
        memUsed: formatBytes(latest.mem_bytes || 0),
        memTotal: formatBytes(latest.mem_limit || 0),
        memPct: latest.mem_limit ? ((latest.mem_bytes / latest.mem_limit) * 100).toFixed(1) : '0',
        netRx: formatBytes(latest.net_rx || 0),
        netTx: formatBytes(latest.net_tx || 0),
        diskRead: formatBytes(latest.disk_read || 0),
        diskWrite: formatBytes(latest.disk_write || 0),
      }
      cpuHistory = metricsData.map((m) => ({ x: new Date(m.timestamp), y: m.cpu_pct }))
      memHistory = metricsData.map((m) => ({
        x: new Date(m.timestamp),
        y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0,
      }))
    }

    // Load per-app metrics and request stats
    const hourAgo = new Date(Date.now() - 3600000).toISOString()
    await Promise.all(
      apps.map(async (app) => {
        const slug = app.Slug || app.slug
        const [mRes, rRes, bRes] = await Promise.all([
          api.appMetrics(slug, hourAgo, now),
          api.appRequests(slug, hourAgo, now),
          api.listBackupRuns(slug),
        ])
        if (mRes.data && mRes.data.length > 0) {
          const latest = mRes.data[mRes.data.length - 1]
          appMetricsMap[slug] = {
            cpu: latest.cpu_pct,
            memPct: latest.mem_limit ? (latest.mem_bytes / latest.mem_limit) * 100 : 0,
          }
        }
        if (rRes.data) {
          appRequestsMap[slug] = rRes.data
        }
        if (bRes.data && bRes.data.length > 0) {
          backupRunsByApp[slug] = bRes.data
        }
      })
    )
    // Trigger reactivity
    appMetricsMap = { ...appMetricsMap }
    appRequestsMap = { ...appRequestsMap }
    backupRunsByApp = { ...backupRunsByApp }

    loading = false
  }

  function formatBytes(bytes) {
    if (!bytes) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
  }

  function formatTime(ts) {
    if (!ts) return ''
    const d = new Date(ts)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  function formatDate(ts) {
    if (!ts) return ''
    return new Date(ts).toLocaleString()
  }

  let runningCount = $derived(apps.filter((a) => a.Status === 'running').length)
  let stoppedCount = $derived(apps.filter((a) => a.Status !== 'running').length)

  let activeAlerts = $derived((alertHistory || []).filter((h) => !h.resolved_at))

  let recentBackups = $derived(() => {
    const all = []
    for (const [slug, runs] of Object.entries(backupRunsByApp)) {
      for (const run of runs.slice(0, 3)) {
        all.push({ ...run, slug })
      }
    }
    return all.sort((a, b) => new Date(b.started_at) - new Date(a.started_at)).slice(0, 5)
  })

  let filteredApps = $derived(() => {
    let result = apps
    if (filterStatus !== 'all') {
      result = result.filter((a) => a.Status === filterStatus)
    }
    if (sortBy === 'name') {
      result = [...result].sort((a, b) => (a.Name || '').localeCompare(b.Name || ''))
    } else if (sortBy === 'status') {
      result = [...result].sort((a, b) => (a.Status || '').localeCompare(b.Status || ''))
    } else if (sortBy === 'cpu') {
      result = [...result].sort((a, b) => {
        const aCpu = appMetricsMap[a.Slug]?.cpu || 0
        const bCpu = appMetricsMap[b.Slug]?.cpu || 0
        return bCpu - aCpu
      })
    }
    return result
  })
</script>

<Layout>
  {#if loading}
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      <Skeleton type="card" count={4} />
    </div>
    <div class="grid grid-cols-3 gap-3 mb-4">
      <Skeleton type="card" count={3} />
    </div>
    <div class="grid grid-cols-2 gap-3">
      <Skeleton type="chart" count={2} />
    </div>
  {:else}
    <!-- System Health -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      <StatCard
        label="CPU"
        value="{latestMetrics?.cpu || '0'}%"
        color={parseFloat(latestMetrics?.cpu || 0) > 80 ? 'text-danger' : parseFloat(latestMetrics?.cpu || 0) > 50 ? 'text-warning' : 'text-success'}
      />
      <StatCard
        label="Memory"
        value="{latestMetrics?.memPct || '0'}%"
        sub="{latestMetrics?.memUsed || '0'} / {latestMetrics?.memTotal || '0'}"
        color={parseFloat(latestMetrics?.memPct || 0) > 80 ? 'text-danger' : parseFloat(latestMetrics?.memPct || 0) > 50 ? 'text-warning' : 'text-success'}
      />
      <StatCard label="Network" value="{latestMetrics?.netRx || '0 B'}/s" sub="TX: {latestMetrics?.netTx || '0 B'}/s" />
      <StatCard label="Disk I/O" value="{latestMetrics?.diskRead || '0 B'}/s" sub="Write: {latestMetrics?.diskWrite || '0 B'}/s" />
    </div>

    <!-- App Summary -->
    <div class="grid grid-cols-3 gap-3 mb-4">
      <StatCard label="Total Apps" value={apps.length} />
      <StatCard label="Running" value={runningCount} color="text-success" />
      <button onclick={() => filterStatus = filterStatus === 'stopped' ? 'all' : 'stopped'} class="text-left">
        <StatCard label="Stopped / Error" value={stoppedCount} color={stoppedCount > 0 ? 'text-danger' : 'text-text-secondary'} />
      </button>
    </div>

    <!-- Main Content: Apps + Sidebar panels -->
    <div class="grid grid-cols-1 xl:grid-cols-5 gap-4 mb-4">
      <!-- Apps Grid (3/5) -->
      <div class="xl:col-span-3">
        <div class="flex items-center justify-between mb-3">
          <h2 class="text-base font-semibold text-text-primary">Applications</h2>
          <div class="flex items-center gap-2">
            <select
              bind:value={filterStatus}
              class="text-xs bg-surface-2 border border-border rounded-md px-2 py-1 text-text-secondary"
            >
              <option value="all">All</option>
              <option value="running">Running</option>
              <option value="stopped">Stopped</option>
              <option value="error">Error</option>
            </select>
            <select
              bind:value={sortBy}
              class="text-xs bg-surface-2 border border-border rounded-md px-2 py-1 text-text-secondary"
            >
              <option value="name">Name</option>
              <option value="status">Status</option>
              <option value="cpu">CPU</option>
            </select>
          </div>
        </div>

        {#if apps.length === 0}
          <div class="bg-surface-2 border border-border rounded-lg p-8 text-center">
            <p class="text-text-secondary text-sm">No apps deployed yet.</p>
          </div>
        {:else}
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            {#each filteredApps() as app}
              <AppCard {app} metrics={appMetricsMap[app.Slug]} />
            {/each}
          </div>
        {/if}
      </div>

      <!-- Side Panels (2/5) -->
      <div class="xl:col-span-2 flex flex-col gap-4">
        <!-- Active Alerts -->
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Active Alerts</h3>
            <a href="#/alerts" class="text-xs text-accent hover:underline">View all</a>
          </div>
          {#if activeAlerts.length === 0}
            <p class="text-xs text-text-secondary">No active alerts</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each activeAlerts.slice(0, 5) as alert}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full bg-danger shrink-0"></span>
                  <span class="text-text-primary">Rule #{alert.rule_id}</span>
                  <span class="text-text-muted ml-auto">{formatTime(alert.fired_at)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Recent Backups -->
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Recent Backups</h3>
            <a href="#/backups" class="text-xs text-accent hover:underline">View all</a>
          </div>
          {#if recentBackups().length === 0}
            <p class="text-xs text-text-secondary">No backup runs</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each recentBackups() as run}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full shrink-0 {run.status === 'completed' ? 'bg-success' : run.status === 'failed' ? 'bg-danger' : 'bg-warning'}"></span>
                  <span class="text-text-primary truncate">{run.slug}</span>
                  <Badge variant={run.status === 'completed' ? 'success' : 'danger'}>{run.status}</Badge>
                  <span class="text-text-muted ml-auto whitespace-nowrap">{formatTime(run.started_at)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Alert History (recent) -->
        <div class="bg-surface-2 border border-border rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold text-text-primary">Alert History</h3>
          </div>
          {#if (alertHistory || []).length === 0}
            <p class="text-xs text-text-secondary">No alerts fired</p>
          {:else}
            <div class="flex flex-col gap-2">
              {#each (alertHistory || []).slice(0, 5) as h}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-1.5 h-1.5 rounded-full shrink-0 {h.resolved_at ? 'bg-success' : 'bg-danger'}"></span>
                  <span class="text-text-primary">Rule #{h.rule_id}</span>
                  <span class="text-text-muted ml-auto">{formatDate(h.fired_at)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Charts -->
    <div class="mb-3 flex items-center justify-between">
      <h2 class="text-base font-semibold text-text-primary">System Trends</h2>
      <div class="flex gap-1">
        {#each Object.keys(rangeMs) as range}
          <button
            onclick={() => { timeRange = range; loadDashboard() }}
            class="px-2 py-1 text-xs rounded-md border transition-colors
              {timeRange === range ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
          >
            {range}
          </button>
        {/each}
      </div>
    </div>
    {#if cpuHistory.length > 0}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <MetricsChart data={cpuHistory} label="CPU Usage" color="#58a6ff" unit="%" />
        <MetricsChart data={memHistory} label="Memory Usage" color="#3fb950" unit="%" />
      </div>
    {:else}
      <div class="bg-surface-2 border border-border rounded-lg p-8 text-center">
        <p class="text-text-secondary text-sm">No metrics data available.</p>
      </div>
    {/if}
  {/if}
</Layout>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/routes/Dashboard.svelte
git commit -m "feat(ui): redesign dashboard with full info panels"
```

---

### Task 11: App Detail Page Redesign

**Files:**
- Modify: `ui/src/routes/AppDetail.svelte`

- [ ] **Step 1: Rewrite AppDetail with Tailwind and enhanced tabs**

Replace `ui/src/routes/AppDetail.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import MetricsChart from '../components/MetricsChart.svelte'
  import LogViewer from '../components/LogViewer.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Badge from '../components/Badge.svelte'
  import Button from '../components/Button.svelte'
  import Modal from '../components/Modal.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'
  import { push } from 'svelte-spa-router'
  import { toasts } from '../lib/stores/toast.js'

  let { params } = $props()
  let slug = $derived(params.slug)

  let app = $state(null)
  let activeTab = $state('overview')
  let metricsRange = $state('1h')
  let cpuData = $state([])
  let memData = $state([])
  let netRxData = $state([])
  let netTxData = $state([])
  let diskReadData = $state([])
  let diskWriteData = $state([])
  let requestStats = $state(null)
  let loading = $state(true)
  let showDeleteModal = $state(false)
  let removing = $state(false)
  let backupConfigs = $state([])
  let backupRuns = $state([])
  let restoreTarget = $state(null)
  let showBackupForm = $state(false)

  // Backup form
  let bStrategy = $state('postgres')
  let bTarget = $state('s3')
  let bCron = $state('0 2 * * *')
  let bRetention = $state(7)

  const tabs = ['overview', 'logs', 'metrics', 'backups']
  const rangeMs = { '1h': 3600000, '6h': 21600000, '24h': 86400000, '7d': 604800000 }

  onMount(loadApp)

  async function loadApp() {
    const res = await api.getApp(slug)
    if (res.error) { push('/'); return }
    app = res.data
    loading = false
    loadRequests()
  }

  async function loadMetrics() {
    const now = new Date().toISOString()
    const from = new Date(Date.now() - rangeMs[metricsRange]).toISOString()
    const res = await api.appMetrics(slug, from, now)
    const data = res.data || []
    cpuData = data.map((m) => ({ x: new Date(m.timestamp), y: m.cpu_pct }))
    memData = data.map((m) => ({ x: new Date(m.timestamp), y: m.mem_limit ? (m.mem_bytes / m.mem_limit) * 100 : 0 }))
    netRxData = data.map((m) => ({ x: new Date(m.timestamp), y: m.net_rx || 0 }))
    netTxData = data.map((m) => ({ x: new Date(m.timestamp), y: m.net_tx || 0 }))
    diskReadData = data.map((m) => ({ x: new Date(m.timestamp), y: m.disk_read || 0 }))
    diskWriteData = data.map((m) => ({ x: new Date(m.timestamp), y: m.disk_write || 0 }))
  }

  async function loadRequests() {
    const now = new Date().toISOString()
    const from = new Date(Date.now() - 3600000).toISOString()
    const res = await api.appRequests(slug, from, now)
    requestStats = res.data
  }

  async function loadBackups() {
    const [cRes, rRes] = await Promise.all([
      api.listBackupConfigs(slug),
      api.listBackupRuns(slug),
    ])
    backupConfigs = cRes.data || []
    backupRuns = rRes.data || []
  }

  async function handleRemove() {
    removing = true
    const res = await api.removeApp(slug)
    removing = false
    showDeleteModal = false
    if (!res.error) push('/')
  }

  async function createBackupConfig() {
    const res = await api.createBackupConfig(slug, {
      strategy: bStrategy, target: bTarget,
      cron_expr: bCron, retention_days: bRetention,
    })
    if (!res.error) { showBackupForm = false; loadBackups() }
  }

  async function deleteBackupConfig(id) {
    await api.deleteBackupConfig(id)
    loadBackups()
  }

  async function triggerBackup() {
    await api.triggerBackup(slug)
    loadBackups()
  }

  async function confirmRestore() {
    if (!restoreTarget) return
    await api.restore(restoreTarget)
    restoreTarget = null
    loadBackups()
  }

  $effect(() => {
    if (activeTab === 'metrics') loadMetrics()
    if (activeTab === 'backups') loadBackups()
  })
</script>

<Layout>
  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" />
      <Skeleton type="card" count={3} />
    </div>
  {:else if app}
    <!-- Header -->
    <div class="mb-6">
      <a href="#/" class="text-xs text-accent hover:underline mb-2 inline-block">&larr; Back to Dashboard</a>
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3">
          <span class="w-3 h-3 rounded-full {app.Status === 'running' ? 'bg-success' : app.Status === 'error' ? 'bg-danger' : 'bg-text-muted'}"></span>
          <h1 class="text-xl font-bold text-text-primary">{app.Name}</h1>
          <Badge variant={app.Status === 'running' ? 'success' : app.Status === 'error' ? 'danger' : 'default'}>{app.Status}</Badge>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="danger" size="sm" onclick={() => showDeleteModal = true}>Delete</Button>
        </div>
      </div>
      {#if app.Domain}
        <a href="https://{app.Domain}" target="_blank" rel="noopener" class="text-sm text-accent hover:underline mt-1 inline-block">{app.Domain}</a>
      {/if}
    </div>

    <!-- Tabs -->
    <div class="flex border-b border-border mb-6">
      {#each tabs as tab}
        <button
          onclick={() => activeTab = tab}
          class="px-4 py-2 text-sm capitalize transition-colors border-b-2
            {activeTab === tab ? 'text-text-primary border-accent' : 'text-text-secondary border-transparent hover:text-text-primary'}"
        >
          {tab}
        </button>
      {/each}
    </div>

    <!-- Tab Content -->
    {#if activeTab === 'overview'}
      <!-- Stats -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
        <StatCard label="Total Requests" value={requestStats?.total ?? 0} />
        <StatCard label="Avg Latency" value="{requestStats?.avg_latency_ms?.toFixed(1) ?? '0'}ms" />
        <StatCard label="Error Rate" value="{requestStats?.error_rate?.toFixed(1) ?? '0'}%"
          color={parseFloat(requestStats?.error_rate || 0) > 5 ? 'text-danger' : 'text-success'} />
        <StatCard label="Status" value={app.Status} color={app.Status === 'running' ? 'text-success' : 'text-danger'} />
      </div>

      <!-- Details -->
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
        <h3 class="text-sm font-semibold text-text-primary mb-3">Details</h3>
        <div class="grid grid-cols-2 gap-4 text-sm">
          <div>
            <span class="text-xs text-text-secondary uppercase tracking-wider">Slug</span>
            <p class="text-text-primary mt-0.5">{app.Slug}</p>
          </div>
          <div>
            <span class="text-xs text-text-secondary uppercase tracking-wider">Status</span>
            <p class="text-text-primary mt-0.5 capitalize">{app.Status}</p>
          </div>
          {#if app.Domain}
            <div>
              <span class="text-xs text-text-secondary uppercase tracking-wider">Domain</span>
              <p class="text-text-primary mt-0.5">{app.Domain}</p>
            </div>
          {/if}
          {#if app.ComposeFile}
            <div>
              <span class="text-xs text-text-secondary uppercase tracking-wider">Compose File</span>
              <p class="text-text-primary mt-0.5 font-mono text-xs">{app.ComposeFile}</p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Labels -->
      {#if app.Labels && Object.keys(app.Labels).length > 0}
        <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
          <h3 class="text-sm font-semibold text-text-primary mb-3">Labels</h3>
          <div class="flex flex-col gap-1">
            {#each Object.entries(app.Labels) as [key, val]}
              <div class="flex gap-2 text-xs px-2 py-1.5 bg-surface-1 rounded">
                <span class="text-text-secondary min-w-48 break-all">{key}</span>
                <span class="text-text-primary break-all">{val}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}

    {:else if activeTab === 'logs'}
      <LogViewer {slug} />

    {:else if activeTab === 'metrics'}
      <div class="flex gap-1 mb-4">
        {#each Object.keys(rangeMs) as range}
          <button
            onclick={() => { metricsRange = range; loadMetrics() }}
            class="px-2 py-1 text-xs rounded-md border transition-colors
              {metricsRange === range ? 'border-accent text-accent' : 'border-border text-text-secondary hover:text-text-primary'}"
          >
            {range}
          </button>
        {/each}
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <MetricsChart data={cpuData} label="CPU Usage" color="#58a6ff" unit="%" />
        <MetricsChart data={memData} label="Memory Usage" color="#3fb950" unit="%" />
        <MetricsChart data={netRxData} label="Network RX" color="#d29922" unit=" B/s" />
        <MetricsChart data={netTxData} label="Network TX" color="#bc8cff" unit=" B/s" />
        <MetricsChart data={diskReadData} label="Disk Read" color="#f78166" unit=" B/s" />
        <MetricsChart data={diskWriteData} label="Disk Write" color="#f85149" unit=" B/s" />
      </div>

    {:else if activeTab === 'backups'}
      <!-- Backup Configs -->
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-text-primary">Backup Configs</h3>
          <div class="flex gap-2">
            <Button size="sm" onclick={triggerBackup}>Run Now</Button>
            <Button size="sm" variant="secondary" onclick={() => showBackupForm = !showBackupForm}>
              {showBackupForm ? 'Cancel' : 'New Config'}
            </Button>
          </div>
        </div>

        {#if showBackupForm}
          <form onsubmit={(e) => { e.preventDefault(); createBackupConfig() }} class="bg-surface-1 rounded-md p-4 mb-4 grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs text-text-secondary mb-1">Strategy</label>
              <select bind:value={bStrategy} class="w-full px-2 py-1.5 bg-input-bg border border-border rounded text-sm text-text-primary">
                <option>postgres</option><option>volume</option>
              </select>
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1">Target</label>
              <select bind:value={bTarget} class="w-full px-2 py-1.5 bg-input-bg border border-border rounded text-sm text-text-primary">
                <option>s3</option><option>local</option>
              </select>
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1">Cron Schedule</label>
              <input bind:value={bCron} class="w-full px-2 py-1.5 bg-input-bg border border-border rounded text-sm text-text-primary" />
            </div>
            <div>
              <label class="block text-xs text-text-secondary mb-1">Retention (days)</label>
              <input type="number" bind:value={bRetention} class="w-full px-2 py-1.5 bg-input-bg border border-border rounded text-sm text-text-primary" />
            </div>
            <div class="col-span-2 flex justify-end">
              <Button type="submit" size="sm">Create</Button>
            </div>
          </form>
        {/if}

        {#if backupConfigs.length === 0}
          <p class="text-xs text-text-secondary">No backup configs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border">
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Strategy</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Target</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Schedule</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Retention</th>
                <th class="py-2 px-3"></th>
              </tr></thead>
              <tbody class="divide-y divide-border-muted">
                {#each backupConfigs as c}
                  <tr class="hover:bg-surface-1">
                    <td class="py-2 px-3">{c.strategy}</td>
                    <td class="py-2 px-3">{c.target}</td>
                    <td class="py-2 px-3 font-mono text-xs">{c.cron_expr}</td>
                    <td class="py-2 px-3">{c.retention_days}d</td>
                    <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => deleteBackupConfig(c.id)}>Delete</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

      <!-- Backup Runs -->
      <div class="bg-surface-2 border border-border rounded-lg p-4">
        <h3 class="text-sm font-semibold text-text-primary mb-3">Backup Runs</h3>
        {#if backupRuns.length === 0}
          <p class="text-xs text-text-secondary">No backup runs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border">
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">ID</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Status</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Started</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Finished</th>
                <th class="py-2 px-3"></th>
              </tr></thead>
              <tbody class="divide-y divide-border-muted">
                {#each backupRuns as r}
                  <tr class="hover:bg-surface-1">
                    <td class="py-2 px-3">{r.id}</td>
                    <td class="py-2 px-3"><Badge variant={r.status === 'completed' ? 'success' : 'danger'}>{r.status}</Badge></td>
                    <td class="py-2 px-3">{r.started_at ? new Date(r.started_at).toLocaleString() : '-'}</td>
                    <td class="py-2 px-3">{r.finished_at ? new Date(r.finished_at).toLocaleString() : '-'}</td>
                    <td class="py-2 px-3"><Button variant="secondary" size="sm" onclick={() => restoreTarget = r.id}>Restore</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}

    {#if showDeleteModal}
      <Modal title="Delete App" message="This will remove {app.Name} and all its data. Are you sure?" onConfirm={handleRemove} onCancel={() => showDeleteModal = false} />
    {/if}

    {#if restoreTarget}
      <Modal title="Confirm Restore" message="This will restore the backup. Are you sure?" onConfirm={confirmRestore} onCancel={() => restoreTarget = null} />
    {/if}
  {/if}
</Layout>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/routes/AppDetail.svelte
git commit -m "feat(ui): redesign app detail page with enhanced tabs"
```

---

### Task 12: Alerts Page Redesign

**Files:**
- Modify: `ui/src/routes/Alerts.svelte`

- [ ] **Step 1: Rewrite Alerts page with Tailwind and slide panels**

Replace `ui/src/routes/Alerts.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let webhooks = $state([])
  let rules = $state([])
  let history = $state([])
  let apps = $state([])
  let loading = $state(true)

  let showWebhookPanel = $state(false)
  let showRulePanel = $state(false)

  // webhook form
  let whName = $state('')
  let whType = $state('slack')
  let whUrl = $state('')

  // rule form
  let rApp = $state('')
  let rMetric = $state('cpu_pct')
  let rOp = $state('>')
  let rThreshold = $state(80)
  let rDuration = $state(60)
  let rWebhook = $state('')

  onMount(loadAll)

  async function loadAll() {
    loading = true
    const [wRes, rRes, hRes, aRes] = await Promise.all([
      api.listWebhooks(),
      api.listAlertRules(),
      api.alertHistory(),
      api.listApps(),
    ])
    webhooks = wRes.data || []
    rules = rRes.data || []
    history = hRes.data || []
    apps = aRes.data || []
    loading = false
  }

  async function createWebhook() {
    const res = await api.createWebhook({ name: whName, type: whType, url: whUrl })
    if (!res.error) { whName = ''; whUrl = ''; showWebhookPanel = false; loadAll() }
  }

  async function delWebhook(id) { await api.deleteWebhook(id); loadAll() }

  async function createRule() {
    const res = await api.createAlertRule({
      app_slug: rApp, metric: rMetric, operator: rOp,
      threshold: +rThreshold, duration_secs: +rDuration, webhook_id: +rWebhook,
    })
    if (!res.error) { showRulePanel = false; loadAll() }
  }

  async function delRule(id) { await api.deleteAlertRule(id); loadAll() }
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">Alerts</h1>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={3} />
    </div>
  {:else}
    <!-- Webhooks -->
    <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Webhooks</h3>
        <Button size="sm" variant="secondary" onclick={() => showWebhookPanel = true}>Add Webhook</Button>
      </div>
      {#if webhooks.length === 0}
        <p class="text-sm text-text-secondary">No webhooks configured.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Name</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Type</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">URL</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each webhooks as w}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3">{w.name}</td>
                  <td class="py-2 px-3"><Badge>{w.type}</Badge></td>
                  <td class="py-2 px-3 max-w-48 truncate text-text-secondary">{w.url}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => delWebhook(w.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Alert Rules -->
    <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Alert Rules</h3>
        <Button size="sm" variant="secondary" onclick={() => showRulePanel = true}>Add Rule</Button>
      </div>
      {#if rules.length === 0}
        <p class="text-sm text-text-secondary">No alert rules.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">App</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Metric</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Condition</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Duration</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Webhook</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each rules as r}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3">{r.app_slug || 'System'}</td>
                  <td class="py-2 px-3"><Badge variant="info">{r.metric}</Badge></td>
                  <td class="py-2 px-3 font-mono text-xs">{r.operator} {r.threshold}</td>
                  <td class="py-2 px-3">{r.duration_secs}s</td>
                  <td class="py-2 px-3">{r.webhook_id}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => delRule(r.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Alert History -->
    <div class="bg-surface-2 border border-border rounded-lg p-4">
      <h3 class="text-sm font-semibold text-text-primary mb-3">Alert History</h3>
      {#if history.length === 0}
        <p class="text-sm text-text-secondary">No alerts fired.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Rule</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Fired</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Resolved</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Status</th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each history as h}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3">#{h.rule_id}</td>
                  <td class="py-2 px-3">{new Date(h.fired_at).toLocaleString()}</td>
                  <td class="py-2 px-3">{h.resolved_at ? new Date(h.resolved_at).toLocaleString() : '-'}</td>
                  <td class="py-2 px-3">
                    <Badge variant={h.resolved_at ? 'success' : 'danger'}>{h.resolved_at ? 'Resolved' : 'Active'}</Badge>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Webhook Slide Panel -->
  <SlidePanel title="Add Webhook" open={showWebhookPanel} onclose={() => showWebhookPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createWebhook() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Name</label>
        <input bind:value={whName} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Type</label>
        <select bind:value={whType} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>slack</option><option>telegram</option><option>discord</option><option>custom</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">URL</label>
        <input bind:value={whUrl} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <Button type="submit">Create Webhook</Button>
    </form>
  </SlidePanel>

  <!-- Rule Slide Panel -->
  <SlidePanel title="Add Alert Rule" open={showRulePanel} onclose={() => showRulePanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createRule() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">App</label>
        <select bind:value={rApp} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option value="">System-wide</option>
          {#each apps as a}<option value={a.Slug || a.slug}>{a.Slug || a.slug}</option>{/each}
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Metric</label>
        <select bind:value={rMetric} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>cpu_pct</option><option>mem_pct</option>
        </select>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-xs text-text-secondary mb-1">Operator</label>
          <select bind:value={rOp} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
            <option value=">">&gt;</option><option value="<">&lt;</option><option value=">=">&gt;=</option><option value="<=">&lt;=</option>
          </select>
        </div>
        <div>
          <label class="block text-xs text-text-secondary mb-1">Threshold</label>
          <input type="number" bind:value={rThreshold} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary" />
        </div>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Duration (seconds)</label>
        <input type="number" bind:value={rDuration} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Webhook</label>
        <select bind:value={rWebhook} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option value="">Select webhook</option>
          {#each webhooks as w}<option value={w.id}>{w.name}</option>{/each}
        </select>
      </div>
      <Button type="submit">Create Rule</Button>
    </form>
  </SlidePanel>
</Layout>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/routes/Alerts.svelte
git commit -m "feat(ui): redesign alerts page with slide panels"
```

---

### Task 13: Backups Page Redesign

**Files:**
- Modify: `ui/src/routes/Backups.svelte`

- [ ] **Step 1: Rewrite Backups page with Tailwind**

Replace `ui/src/routes/Backups.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import Modal from '../components/Modal.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let apps = $state([])
  let selectedApp = $state('')
  let configs = $state([])
  let runs = $state([])
  let loading = $state(true)
  let restoreTarget = $state(null)
  let showConfigPanel = $state(false)

  // form
  let strategy = $state('postgres')
  let target = $state('s3')
  let cron = $state('0 2 * * *')
  let retention = $state(7)

  onMount(async () => {
    const res = await api.listApps()
    apps = res.data || []
    loading = false
  })

  async function loadAppData() {
    if (!selectedApp) { configs = []; runs = []; return }
    const [cRes, rRes] = await Promise.all([
      api.listBackupConfigs(selectedApp),
      api.listBackupRuns(selectedApp),
    ])
    configs = cRes.data || []
    runs = rRes.data || []
  }

  async function createConfig() {
    const res = await api.createBackupConfig(selectedApp, {
      strategy, target, cron_expr: cron, retention_days: retention,
    })
    if (!res.error) { showConfigPanel = false; loadAppData() }
  }

  async function deleteConfig(id) {
    await api.deleteBackupConfig(id)
    loadAppData()
  }

  async function backupNow() {
    await api.triggerBackup(selectedApp)
    loadAppData()
  }

  async function confirmRestore() {
    if (!restoreTarget) return
    await api.restore(restoreTarget)
    restoreTarget = null
    loadAppData()
  }

  function onAppChange(e) {
    selectedApp = e.target.value
    loadAppData()
  }
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">Backups</h1>
  </div>

  {#if loading}
    <Skeleton type="card" count={2} />
  {:else}
    <!-- App Selector -->
    <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
      <label class="block text-xs text-text-secondary mb-1.5">Select App</label>
      <select
        value={selectedApp}
        onchange={onAppChange}
        class="w-full max-w-xs px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary"
      >
        <option value="">-- choose app --</option>
        {#each apps as app}<option value={app.Slug || app.slug}>{app.Name || app.Slug || app.slug}</option>{/each}
      </select>
    </div>

    {#if selectedApp}
      <!-- Backup Configs -->
      <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-text-primary">Backup Configs</h3>
          <Button size="sm" variant="secondary" onclick={() => showConfigPanel = true}>New Config</Button>
        </div>
        {#if configs.length === 0}
          <p class="text-sm text-text-secondary">No backup configs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border">
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Strategy</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Target</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Cron</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Retention</th>
                <th class="py-2 px-3"></th>
              </tr></thead>
              <tbody class="divide-y divide-border-muted">
                {#each configs as c}
                  <tr class="hover:bg-surface-1">
                    <td class="py-2 px-3">{c.strategy}</td>
                    <td class="py-2 px-3">{c.target}</td>
                    <td class="py-2 px-3 font-mono text-xs">{c.cron_expr}</td>
                    <td class="py-2 px-3">{c.retention_days}d</td>
                    <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => deleteConfig(c.id)}>Delete</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

      <!-- Backup Runs -->
      <div class="bg-surface-2 border border-border rounded-lg p-4">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-text-primary">Backup Runs</h3>
          <Button size="sm" onclick={backupNow}>Backup Now</Button>
        </div>
        {#if runs.length === 0}
          <p class="text-sm text-text-secondary">No backup runs.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead><tr class="border-b border-border">
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">ID</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Status</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Started</th>
                <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Finished</th>
                <th class="py-2 px-3"></th>
              </tr></thead>
              <tbody class="divide-y divide-border-muted">
                {#each runs as r}
                  <tr class="hover:bg-surface-1">
                    <td class="py-2 px-3">{r.id}</td>
                    <td class="py-2 px-3"><Badge variant={r.status === 'completed' ? 'success' : 'danger'}>{r.status}</Badge></td>
                    <td class="py-2 px-3">{r.started_at ? new Date(r.started_at).toLocaleString() : '-'}</td>
                    <td class="py-2 px-3">{r.finished_at ? new Date(r.finished_at).toLocaleString() : '-'}</td>
                    <td class="py-2 px-3"><Button variant="secondary" size="sm" onclick={() => restoreTarget = r.id}>Restore</Button></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}
  {/if}

  {#if restoreTarget}
    <Modal title="Confirm Restore" message="This will restore the backup. Are you sure?" onConfirm={confirmRestore} onCancel={() => restoreTarget = null} />
  {/if}

  <!-- New Config Slide Panel -->
  <SlidePanel title="New Backup Config" open={showConfigPanel} onclose={() => showConfigPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createConfig() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Strategy</label>
        <select bind:value={strategy} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>postgres</option><option>volume</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Target</label>
        <select bind:value={target} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>s3</option><option>local</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Cron Schedule</label>
        <input bind:value={cron} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Retention (days)</label>
        <input type="number" bind:value={retention} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary" />
      </div>
      <Button type="submit">Create Config</Button>
    </form>
  </SlidePanel>
</Layout>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/routes/Backups.svelte
git commit -m "feat(ui): redesign backups page with slide panel"
```

---

### Task 14: Users Page Redesign

**Files:**
- Modify: `ui/src/routes/Users.svelte`

- [ ] **Step 1: Rewrite Users page with Tailwind and slide panels**

Replace `ui/src/routes/Users.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Badge from '../components/Badge.svelte'
  import SlidePanel from '../components/SlidePanel.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let users = $state([])
  let keys = $state([])
  let newKey = $state('')
  let loading = $state(true)

  let showUserPanel = $state(false)
  let showKeyPanel = $state(false)

  // user form
  let uName = $state('')
  let uPass = $state('')
  let uRole = $state('viewer')

  // key form
  let kName = $state('')

  const roleVariants = {
    super_admin: 'danger',
    admin: 'warning',
    viewer: 'info',
  }

  onMount(loadAll)

  async function loadAll() {
    loading = true
    const [uRes, kRes] = await Promise.all([
      api.listUsers(),
      api.listAPIKeys(),
    ])
    users = uRes.data || []
    keys = kRes.data || []
    loading = false
  }

  async function createUser() {
    const res = await api.createUser({ username: uName, password: uPass, role: uRole })
    if (!res.error) { uName = ''; uPass = ''; showUserPanel = false; loadAll() }
  }

  async function delUser(id) { await api.deleteUser(id); loadAll() }

  async function createKey() {
    newKey = ''
    const res = await api.createAPIKey(kName)
    if (!res.error) {
      newKey = res.data?.key || ''
      kName = ''
      showKeyPanel = false
      loadAll()
    }
  }

  async function revokeKey(id) { await api.deleteAPIKey(id); loadAll() }

  function copyKey() {
    navigator.clipboard.writeText(newKey)
  }
</script>

<Layout>
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-lg font-bold text-text-primary">Users & API Keys</h1>
  </div>

  {#if loading}
    <div class="space-y-4">
      <Skeleton type="card" count={2} />
    </div>
  {:else}
    <!-- New Key Display -->
    {#if newKey}
      <div class="bg-green-900/20 border border-success rounded-lg px-4 py-3 mb-4 light:bg-green-50">
        <p class="text-xs text-success mb-2">New key created (copy now, shown once):</p>
        <div class="flex items-center gap-2">
          <code class="flex-1 text-xs bg-surface-1 text-text-primary px-3 py-2 rounded break-all font-mono">{newKey}</code>
          <Button size="sm" variant="secondary" onclick={copyKey}>Copy</Button>
        </div>
      </div>
    {/if}

    <!-- Users -->
    <div class="bg-surface-2 border border-border rounded-lg p-4 mb-4">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">Users</h3>
        <Button size="sm" variant="secondary" onclick={() => showUserPanel = true}>Add User</Button>
      </div>
      {#if users.length === 0}
        <p class="text-sm text-text-secondary">No users.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">ID</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Username</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Role</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Created</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each users as u}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3">{u.id}</td>
                  <td class="py-2 px-3 font-medium">{u.username}</td>
                  <td class="py-2 px-3"><Badge variant={roleVariants[u.role] || 'default'}>{u.role}</Badge></td>
                  <td class="py-2 px-3 text-text-secondary">{u.created_at ? new Date(u.created_at).toLocaleDateString() : ''}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => delUser(u.id)}>Delete</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- API Keys -->
    <div class="bg-surface-2 border border-border rounded-lg p-4">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-text-primary">API Keys</h3>
        <Button size="sm" variant="secondary" onclick={() => showKeyPanel = true}>Create Key</Button>
      </div>
      {#if keys.length === 0}
        <p class="text-sm text-text-secondary">No API keys.</p>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-border">
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Name</th>
              <th class="text-left text-xs font-medium text-text-secondary py-2 px-3">Created</th>
              <th class="py-2 px-3"></th>
            </tr></thead>
            <tbody class="divide-y divide-border-muted">
              {#each keys as k}
                <tr class="hover:bg-surface-1">
                  <td class="py-2 px-3 font-medium">{k.name}</td>
                  <td class="py-2 px-3 text-text-secondary">{new Date(k.created_at).toLocaleString()}</td>
                  <td class="py-2 px-3"><Button variant="danger" size="sm" onclick={() => revokeKey(k.id)}>Revoke</Button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Add User Slide Panel -->
  <SlidePanel title="Add User" open={showUserPanel} onclose={() => showUserPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createUser() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Username</label>
        <input bind:value={uName} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Password</label>
        <input type="password" bind:value={uPass} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <div>
        <label class="block text-xs text-text-secondary mb-1">Role</label>
        <select bind:value={uRole} class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary">
          <option>viewer</option><option>admin</option><option>super_admin</option>
        </select>
      </div>
      <Button type="submit">Create User</Button>
    </form>
  </SlidePanel>

  <!-- Create Key Slide Panel -->
  <SlidePanel title="Create API Key" open={showKeyPanel} onclose={() => showKeyPanel = false}>
    <form onsubmit={(e) => { e.preventDefault(); createKey() }} class="flex flex-col gap-4">
      <div>
        <label class="block text-xs text-text-secondary mb-1">Key Name</label>
        <input bind:value={kName} required class="w-full px-3 py-2 bg-input-bg border border-border rounded-md text-sm text-text-primary focus:ring-2 focus:ring-accent/50" />
      </div>
      <Button type="submit">Create Key</Button>
    </form>
  </SlidePanel>
</Layout>
```

- [ ] **Step 2: Verify build**

```bash
cd ui && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/routes/Users.svelte
git commit -m "feat(ui): redesign users page with slide panels"
```

---

### Task 15: Final Cleanup and Build Verification

**Files:**
- Modify: `ui/src/svelte.config.js` (no changes needed if build works)

- [ ] **Step 1: Run full build and verify no errors**

```bash
cd ui && npm run build
```

Expected: Build succeeds with no errors. All Svelte components compile. Tailwind generates CSS.

- [ ] **Step 2: Check output size**

```bash
ls -la ui/dist/assets/
```

Expected: JS bundle + CSS file in dist/assets/.

- [ ] **Step 3: Rebuild Go binary with new UI**

```bash
make build
```

Expected: Go binary builds successfully with embedded UI.

- [ ] **Step 4: Commit any remaining changes**

```bash
git add -A && git status
```

If there are unstaged changes, commit them:

```bash
git commit -m "chore(ui): final cleanup after redesign"
```
