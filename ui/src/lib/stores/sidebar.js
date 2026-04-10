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
