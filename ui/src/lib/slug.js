// App slugs as accepted by the backend (validAppName in internal/api/deploy.go): alnum first char, then up to 62 of
// alnum, dot, underscore or dash. Route params are validated against this
// before any API call so crafted URLs never reach request paths.
export const SLUG_RE = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$/

export function isValidSlug(slug) {
  return typeof slug === 'string' && SLUG_RE.test(slug)
}
