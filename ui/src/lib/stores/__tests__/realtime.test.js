import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// Minimal WebSocket mock: tracks instances and lets tests drive open/message/close.
class MockSocket {
  static instances = []
  constructor(url) {
    this.url = url
    this.readyState = 0
    this.sent = []
    this.listeners = { open: [], message: [], close: [], error: [] }
    MockSocket.instances.push(this)
  }
  addEventListener(type, fn) { this.listeners[type].push(fn) }
  send(data) { this.sent.push(data) }
  close() { this.readyState = 3; this._fire('close', {}) }
  _fire(type, ev) { for (const fn of this.listeners[type]) fn(ev) }
  open() { this.readyState = 1; this._fire('open', {}) }
  message(obj) { this._fire('message', { data: JSON.stringify(obj) }) }
}

let realtime

beforeEach(async () => {
  vi.resetModules()
  MockSocket.instances = []
  globalThis.WebSocket = MockSocket
  // Provide a location for wsURL()
  Object.defineProperty(globalThis, 'location', {
    value: { protocol: 'http:', host: 'localhost:1234' },
    configurable: true,
    writable: true,
  })
  const mod = await import('../realtime.svelte.js?t=' + Math.random())
  realtime = mod.realtime
})

afterEach(() => {
  realtime?._resetForTests?.()
  vi.useRealTimers()
})

function lastSocket() { return MockSocket.instances[MockSocket.instances.length - 1] }

describe('realtime store', () => {
  it('opens a single socket on first register and queues sub frame', async () => {
    const fn = vi.fn()
    realtime.register('app:foo', fn)
    expect(MockSocket.instances.length).toBe(1)
    const s = lastSocket()
    s.open()
    // Should have sent a sub frame after opening.
    const subs = s.sent.map((x) => JSON.parse(x))
    expect(subs).toEqual(expect.arrayContaining([{ op: 'sub', topic: 'app:foo' }]))
  })

  it('fans out matching frames to all registered fns', async () => {
    const a = vi.fn()
    const b = vi.fn()
    realtime.register('global:apps', a)
    realtime.register('global:apps', b)
    const s = lastSocket()
    s.open()
    // Initial open triggers a synthetic resync that calls all fns once.
    a.mockClear(); b.mockClear()
    s.message({ type: 'app.status', topic: 'global:apps' })
    expect(a).toHaveBeenCalledTimes(1)
    expect(b).toHaveBeenCalledTimes(1)
  })

  it('does not fan out frames for unrelated topics', async () => {
    const fn = vi.fn()
    realtime.register('app:foo', fn)
    const s = lastSocket()
    s.open()
    fn.mockClear()
    s.message({ type: 'x', topic: 'app:other' })
    expect(fn).not.toHaveBeenCalled()
  })

  it('sends unsub when last fn for a topic unregisters', async () => {
    const fn = vi.fn()
    const off = realtime.register('app:foo', fn)
    const s = lastSocket()
    s.open()
    s.sent = []
    off()
    expect(s.sent.map((x) => JSON.parse(x))).toEqual([{ op: 'unsub', topic: 'app:foo' }])
  })

  it('does not unsub while other fns remain', async () => {
    const a = vi.fn(), b = vi.fn()
    const offA = realtime.register('t', a)
    realtime.register('t', b)
    const s = lastSocket()
    s.open()
    s.sent = []
    offA()
    expect(s.sent.length).toBe(0)
  })

  it('resync frame triggers all registered fns', async () => {
    const a = vi.fn(), b = vi.fn()
    realtime.register('app:foo', a)
    realtime.register('global:apps', b)
    const s = lastSocket()
    s.open()
    a.mockClear(); b.mockClear()
    s.message({ type: 'resync', topic: '*' })
    expect(a).toHaveBeenCalledTimes(1)
    expect(b).toHaveBeenCalledTimes(1)
  })

  it('manual invalidate fires registered fns for that topic', async () => {
    const a = vi.fn()
    realtime.register('app:foo', a)
    a.mockClear()
    realtime.invalidate('app:foo')
    expect(a).toHaveBeenCalledTimes(1)
  })

  it('reconnects after socket close and resubscribes', async () => {
    vi.useFakeTimers()
    const fn = vi.fn()
    realtime.register('app:foo', fn)
    let s = lastSocket()
    s.open()
    s.close()
    // Run reconnect timer.
    vi.advanceTimersByTime(2000)
    expect(MockSocket.instances.length).toBe(2)
    s = lastSocket()
    s.open()
    // After reopen, sub frame for app:foo is sent again.
    const sent = s.sent.map((x) => JSON.parse(x))
    expect(sent).toEqual(expect.arrayContaining([{ op: 'sub', topic: 'app:foo' }]))
  })
})
