import { writable } from 'svelte/store'

let nextId = 0

function createToastStore() {
  const { subscribe, update } = writable([])

  function add(type, message, timeout = 4000) {
    const id = nextId++
    update((toasts) => [...toasts, { id, type, message }])
    if (timeout > 0) {
      setTimeout(() => remove(id), timeout)
    }
    return id
  }

  function remove(id) {
    update((toasts) => toasts.filter((t) => t.id !== id))
  }

  return {
    subscribe,
    success: (msg) => add('success', msg),
    // Multi-line errors carry refusal reasons; give people time to read them.
    error: (msg) => add('error', msg, typeof msg === 'string' && msg.includes('\n') ? 12000 : 4000),
    warning: (msg) => add('warning', msg),
    info: (msg) => add('info', msg),
    remove,
  }
}

export const toasts = createToastStore()
