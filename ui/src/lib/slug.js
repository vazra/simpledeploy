// App slugs as accepted by the backend (validAppName in internal/api/deploy.go): alnum first char, then up to 62 of
// alnum, dot, underscore or dash. Route params are validated against this
// before any API call so crafted URLs never reach request paths. It stays
// looser than APP_NAME_RE so apps named before that rule still open.
export const SLUG_RE = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$/

export function isValidSlug(slug) {
  return typeof slug === 'string' && SLUG_RE.test(slug)
}

// Names for new apps (validNewAppName in internal/api/deploy.go). The app runs
// as docker compose project `simpledeploy-<name>`, and compose only accepts
// lowercase letters, digits, hyphens and underscores there.
export const APP_NAME_RE = /^[a-z0-9][a-z0-9_-]{0,62}$/

export const APP_NAME_HINT = 'Lowercase letters, numbers, hyphens (-) and underscores (_). Max 63 characters.'

// appNameError returns a human-readable problem with a new app name, or ''.
export function appNameError(name) {
  const v = typeof name === 'string' ? name : ''
  if (!v.trim()) return 'App name is required'
  if (APP_NAME_RE.test(v)) return ''
  if (v.length > 63) return 'Keep the name to 63 characters or fewer'
  if (APP_NAME_RE.test(v.toLowerCase())) return `Use lowercase letters only, for example "${v.toLowerCase()}"`
  if (!/^[a-zA-Z0-9]/.test(v)) return 'Start the name with a letter or number'
  return 'Use only lowercase letters, numbers, hyphens (-) and underscores (_). Dots and spaces are not allowed.'
}
