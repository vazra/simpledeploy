// Realtime notify-only WebSocket client. Single persistent socket per tab.
// Components register (topic, refetchFn) pairs via realtime.register; on every
// matching server frame the registered fns run. No payload data flows; the
// REST API stays the source of truth.

let socket = null
let status = 'closed' // 'connecting' | 'open' | 'closed'
let topics = new Map() // topic -> Set<refetchFn>
let pendingOps = []   // queued frames until socket opens
let backoffMs = 1000
let reconnectTimer = null
let manuallyClosed = false

const MAX_BACKOFF = 30000
const BACKOFFS = [1000, 2000, 5000, 10000, 30000]
let backoffIdx = 0

function wsURL() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/api/events`
}

function send(obj) {
  const data = JSON.stringify(obj)
  if (socket && socket.readyState === 1) {
    socket.send(data)
  } else {
    pendingOps.push(obj)
  }
}

function flushPending() {
  if (!socket || socket.readyState !== 1) return
  const ops = pendingOps
  pendingOps = []
  for (const o of ops) socket.send(JSON.stringify(o))
}

function fanout(topic) {
  const set = topics.get(topic)
  if (!set) return
  for (const fn of set) {
    try { fn() } catch (e) { /* swallow refetch errors */ }
  }
}

function resyncAll() {
  for (const set of topics.values()) {
    for (const fn of set) {
      try { fn() } catch (e) { /* ignore */ }
    }
  }
}

function connect() {
  if (typeof WebSocket === 'undefined') return
  if (socket && (socket.readyState === 0 || socket.readyState === 1)) return
  manuallyClosed = false
  status = 'connecting'
  let s
  try {
    s = new WebSocket(wsURL())
  } catch (e) {
    scheduleReconnect()
    return
  }
  socket = s
  s.addEventListener('open', () => {
    status = 'open'
    backoffIdx = 0
    backoffMs = BACKOFFS[0]
    // Re-subscribe to every active topic.
    for (const t of topics.keys()) {
      pendingOps.push({ op: 'sub', topic: t })
    }
    flushPending()
    // Synthetic resync: prompt every registered fn to refetch.
    resyncAll()
  })
  s.addEventListener('message', (ev) => {
    let f
    try { f = JSON.parse(ev.data) } catch { return }
    if (f.op === 'pong' || f.op === 'err') return
    if (f.type === 'resync') { resyncAll(); return }
    if (f.topic) fanout(f.topic)
  })
  s.addEventListener('close', () => {
    status = 'closed'
    socket = null
    if (!manuallyClosed) scheduleReconnect()
  })
  s.addEventListener('error', () => {
    try { s.close() } catch {}
  })
}

function scheduleReconnect() {
  if (reconnectTimer) return
  const delay = BACKOFFS[Math.min(backoffIdx, BACKOFFS.length - 1)]
  backoffIdx++
  backoffMs = Math.min(delay, MAX_BACKOFF)
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connect()
  }, backoffMs)
}

function register(topic, fn) {
  if (!topic || typeof fn !== 'function') return () => {}
  let set = topics.get(topic)
  const isFirst = !set
  if (!set) {
    set = new Set()
    topics.set(topic, set)
  }
  set.add(fn)
  if (isFirst) send({ op: 'sub', topic })
  if (!socket) connect()
  return () => unregister(topic, fn)
}

function unregister(topic, fn) {
  const set = topics.get(topic)
  if (!set) return
  set.delete(fn)
  if (set.size === 0) {
    topics.delete(topic)
    send({ op: 'unsub', topic })
  }
}

function invalidate(topic) {
  fanout(topic)
}

// Test-only reset hook. Not part of the public API surface for components.
function _resetForTests() {
  if (socket) { try { socket.close() } catch {} }
  socket = null
  status = 'closed'
  topics = new Map()
  pendingOps = []
  backoffIdx = 0
  if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null }
}

export const realtime = {
  register,
  unregister,
  invalidate,
  get status() { return status },
  _resetForTests,
}
