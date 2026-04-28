// Pure helpers for role-based UI gating. Keep this side-effect free so it can
// be unit-tested without hitting the API.

export function isSuperAdmin(me) {
  return !!me && me.role === 'super_admin'
}

export function isManage(me) {
  return !!me && me.role === 'manage'
}

export function isViewer(me) {
  return !!me && me.role === 'viewer'
}

// Returns true if the user can mutate the given app slug. super_admin can
// mutate any app; manage requires explicit access; viewer never can.
export function canMutateApp(me, slug) {
  if (!me) return false
  if (me.role === 'super_admin') return true
  if (me.role !== 'manage') return false
  if (!slug) return false
  const access = Array.isArray(me.app_access) ? me.app_access : []
  return access.includes(slug)
}

// Returns true if the user can view the given app slug.
export function canViewApp(me, slug) {
  if (!me) return false
  if (me.role === 'super_admin') return true
  if (!slug) return false
  const access = Array.isArray(me.app_access) ? me.app_access : []
  return access.includes(slug)
}
