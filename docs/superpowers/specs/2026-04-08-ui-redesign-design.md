# SimpleDeploy UI Redesign Spec

## Overview

Dashboard-centric redesign of the Svelte SPA. Migrate from custom CSS to Tailwind CSS v4, build reusable component library, redesign dashboard as information-rich command center, add collapsible sidebar, system-preference theming (dark/light/auto), toast + inline notifications, loading states, and accessibility improvements.

## Decisions

- CSS: Tailwind CSS v4 (Vite plugin)
- Audience: Mixed (devs + less-technical users)
- Theme: System-preference auto with manual override (system/dark/light), persisted in localStorage
- Navigation: Collapsible sidebar (full/icons-only), state persisted in localStorage
- Notifications: Toast (top-right, auto-dismiss 4s) + inline form validation
- Dashboard info: All available data surfaced (system metrics, per-app resources, request stats, backup status, alert status, disk/network, activity feed)

## 1. Design System & Shared Infrastructure

### Tailwind Setup
- Tailwind CSS v4 with Vite plugin
- Custom color tokens via CSS variables for theming (--color-surface, --color-text-primary, --color-accent, etc.)
- Dark/light via prefers-color-scheme + manual toggle in localStorage
- Remove all existing custom CSS, replace with Tailwind utility classes

### Reusable Components

New shared components in `ui/src/components/`:

| Component | Purpose |
|-----------|---------|
| Badge.svelte | Status badges (running/stopped/error) with color variants |
| Button.svelte | Primary, secondary, danger, ghost variants + loading spinner |
| StatCard.svelte | Icon + label + value + optional sparkline/trend |
| DataTable.svelte | Sortable table with empty state message |
| Toast.svelte | Notification popup (success/error/warning/info) |
| Skeleton.svelte | Loading placeholder for cards, tables, charts |
| Sidebar.svelte | Extracted from Layout, collapsible, persisted state |
| ThemeToggle.svelte | System/dark/light switcher icon button |
| SlidePanel.svelte | Slide-out panel for create/edit forms |

### Toast Store (`lib/toastStore.js`)
- Svelte writable store, array of {id, type, message, timeout}
- `addToast(type, message)` helper
- Auto-remove after timeout (default 4s)

### API Client Updates (`lib/api.js`)
- All calls return consistent `{data, error}` shape
- Global error handler feeds into toast store
- Loading state helpers

## 2. Dashboard

### Top Row - System Health (4 stat cards)
- CPU: current %, sparkline, green/yellow/red by threshold
- Memory: current % + human-readable used/total (e.g. "2.1 / 4 GB"), sparkline
- Disk I/O: read/write rates
- Network: rx/tx rates

### Second Row - App Summary (3 stat cards)
- Total apps count
- Running count (green)
- Stopped/Error count (red/yellow), clickable to filtered view

### Third Row - Two Column Layout

**Left (~60%) - Applications Grid:**
- Richer app cards: name, status badge, domain link, CPU/memory mini-bars, request count, error rate, last deploy time
- Clickable to app detail
- Sort/filter controls (by status, name, resource usage)

**Right (~40%) - Panels:**
- Active Alerts: firing alerts with severity, metric, app name. "View all" link
- Recent Backups: last 5 runs with status icon, app, timestamp. "View all" link
- Activity Feed: recent events (deploys, restarts, config changes), timestamped, scrollable, ~10 items

### Fourth Row - Full Width Charts
- CPU and Memory over time
- Time range selector: 1h / 6h / 24h / 7d
- Proper legends, Tailwind-styled

All sections show skeleton loaders while fetching. Empty states with helpful messages.

## 3. App Detail Page

### Header
- App name + status badge + domain link (opens new tab)
- Action buttons: Restart, Stop/Start, Delete (with confirmation modal)
- Last deployed timestamp

### Tabs

**Overview:**
- 4 stat cards: CPU%, Memory%, Request count (last hour), Error rate
- Request stats: total, avg latency, p95 latency, error rate
- Per-service resource breakdown table (multi-container)
- Domain/proxy info

**Logs:**
- WebSocket log viewer, restyled with Tailwind
- Service filter dropdown
- Severity color coding (stdout gray, stderr red)
- Timestamp toggle, download button

**Metrics:**
- Time range selector (1h/6h/24h/7d)
- 2-column chart grid: CPU, Memory, Network I/O, Disk I/O, Request latency, Error rate

**Backups:**
- Configs table with create/delete
- Runs table with status, size, timestamp, restore button
- Manual "Run Now" button
- Inline form validation

## 4. Other Pages

### Collapsible Sidebar
- Expanded: icon + label
- Collapsed: icon only, tooltip on hover
- Toggle button at bottom
- Persisted in localStorage
- Nav: Dashboard, Backups, Alerts, Users
- Active route highlighted
- Logo at top (collapses to icon)
- User info + logout + theme toggle at bottom

### Alerts Page
- Two sections: Alert Rules (top) + Alert History (bottom)
- Rules table: app, metric, condition, webhook target, enabled toggle, edit/delete
- History table: rule, fired at, resolved at, value, duration
- Create/edit via slide-out panel
- Inline validation

### Backups Page (global)
- All configs across apps, grouped by app
- Recent runs with status indicators
- Filter by app, status

### Users Page
- Users table: username, role badge, created, app access, delete
- API keys table: name, created, expires, delete
- Create user/key via slide-out panels

### Login Page
- Centered card, clean form
- Setup mode detection
- Inline error messages

## 5. UX Polish

### Loading States
- Skeleton placeholders for cards, tables, charts
- Button loading spinners during async actions
- Page-level loading on initial fetch

### Toast Notifications
- Top-right, stacked
- Types: success (green), error (red), warning (yellow), info (blue)
- Auto-dismiss 4s, manual dismiss via X
- Triggered on: actions, API errors, warnings

### Inline Feedback
- Form validation errors below fields
- Confirmation modals for destructive actions

### Responsive
- Sidebar auto-collapses < 1024px
- Dashboard single-column on mobile
- Tables scroll horizontally on small screens
- Desktop is primary target

### Transitions
- Sidebar collapse/expand animated
- Toast slide-in/fade-out
- Theme switch instant (no color transition)

### Accessibility
- Focus management on modals and slide panels
- Keyboard navigation for sidebar, tabs, tables
- ARIA labels on icon-only buttons
- WCAG AA color contrast in both themes
