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
