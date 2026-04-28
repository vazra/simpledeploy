import { describe, it, expect } from 'vitest'
import { isSuperAdmin, isManage, isViewer, canMutateApp, canViewApp } from '../auth.js'

describe('auth helpers', () => {
  it('role flags', () => {
    expect(isSuperAdmin({ role: 'super_admin' })).toBe(true)
    expect(isSuperAdmin({ role: 'manage' })).toBe(false)
    expect(isManage({ role: 'manage' })).toBe(true)
    expect(isViewer({ role: 'viewer' })).toBe(true)
    expect(isSuperAdmin(null)).toBe(false)
    expect(isManage(undefined)).toBe(false)
  })

  it('canMutateApp truth table', () => {
    const sa = { role: 'super_admin' }
    expect(canMutateApp(sa, 'foo')).toBe(true)
    expect(canMutateApp(sa, null)).toBe(true)

    const mgrWith = { role: 'manage', app_access: ['a', 'b'] }
    expect(canMutateApp(mgrWith, 'a')).toBe(true)
    expect(canMutateApp(mgrWith, 'c')).toBe(false)
    expect(canMutateApp(mgrWith, null)).toBe(false)

    const mgrEmpty = { role: 'manage' }
    expect(canMutateApp(mgrEmpty, 'a')).toBe(false)

    const viewer = { role: 'viewer', app_access: ['a'] }
    expect(canMutateApp(viewer, 'a')).toBe(false)

    expect(canMutateApp(null, 'a')).toBe(false)
  })

  it('canViewApp truth table', () => {
    expect(canViewApp({ role: 'super_admin' }, 'x')).toBe(true)
    expect(canViewApp({ role: 'manage', app_access: ['x'] }, 'x')).toBe(true)
    expect(canViewApp({ role: 'manage', app_access: [] }, 'x')).toBe(false)
    expect(canViewApp({ role: 'viewer', app_access: ['x'] }, 'x')).toBe(true)
    expect(canViewApp({ role: 'viewer' }, 'x')).toBe(false)
  })
})
