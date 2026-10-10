// Pure helpers for API error payloads. Kept separate from api.js so
// components can use them even when api.js is mocked in tests.

// Refused compose/env/endpoint/rollback/deploy requests answer with
// {"error": "...", "violations": ["...", ...]}. Fold the violations into the
// error text so every caller that shows res.error also shows why.
export function violationsOf(data) {
  if (!data || typeof data !== 'object' || !Array.isArray(data.violations)) return []
  return data.violations.filter((v) => typeof v === 'string' && v.trim() !== '')
}

export function formatApiError(data, fallback) {
  let error = fallback
  if (data && typeof data === 'object' && typeof data.error === 'string' && data.error) {
    error = data.error
  }
  const violations = violationsOf(data)
  if (violations.length) {
    error = String(error || 'Request refused').replace(/[\s.:]+$/, '') + ':\n- ' + violations.join('\n- ')
  }
  return error
}

// Split an error (possibly carrying refusal reasons) into display lines.
export function errorLines(error) {
  return String(error ?? '').split('\n').map((l) => l.trimEnd()).filter((l) => l.trim() !== '')
}
