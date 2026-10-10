// Helpers for .env entries returned by GET /api/apps/{slug}/env.
// Viewers receive { key, value: '', masked: true }: the key is visible but the
// value is withheld. A masked entry must never be written back, since its
// empty value would overwrite the real secret.

export const MASKED_ENV_PLACEHOLDER = '•••••• (hidden for viewers)'

export function isMaskedEnv(v) {
  return !!(v && v.masked)
}

export function hasMaskedEnv(vars) {
  return Array.isArray(vars) && vars.some(isMaskedEnv)
}

// Display value for an entry: placeholder for masked entries, raw otherwise.
export function envDisplayValue(v) {
  return isMaskedEnv(v) ? MASKED_ENV_PLACEHOLDER : (v?.value ?? '')
}

// Wire payload for PUT: only key/value. Callers must refuse to save when
// hasMaskedEnv() is true (PUT replaces the whole file).
export function toEnvPayload(vars) {
  return (vars || []).map((v) => ({ key: v.key, value: v.value }))
}
