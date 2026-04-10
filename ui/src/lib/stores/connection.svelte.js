import { api } from '../api.js'

let connected = $state(true)
let checking = false
let listeners = []

async function check() {
  if (checking) return
  checking = true
  const wasConnected = connected
  const res = await api.health()
  connected = !res.error
  if (!wasConnected && connected) {
    listeners.forEach(fn => fn())
  }
  checking = false
}

setInterval(check, 15000)
check()

export const connection = {
  get connected() { return connected },
  check,
  onReconnect(fn) {
    listeners.push(fn)
    return () => { listeners = listeners.filter(l => l !== fn) }
  },
}
