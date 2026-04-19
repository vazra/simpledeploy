import { vi } from 'vitest';

// Mock svelte-spa-router for tests; import { routerMocks } into `vi.mock(...)`.
export function routerMocks() {
  return {
    push: vi.fn(),
    pop: vi.fn(),
    replace: vi.fn(),
    link: (node) => node,
    location: { subscribe: (fn) => { fn('/'); return () => {}; } },
    querystring: { subscribe: (fn) => { fn(''); return () => {}; } },
    params: { subscribe: (fn) => { fn({}); return () => {}; } },
    default: () => null,
  };
}
