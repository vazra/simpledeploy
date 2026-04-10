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
    error: (msg) => add('error', msg),
    warning: (msg) => add('warning', msg),
    info: (msg) => add('info', msg),
    remove,
  }
}

export const toasts = createToastStore()
