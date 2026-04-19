import { createRawSnippet } from 'svelte';

// Build a Svelte 5 snippet from a string or an HTML-producing function.
// Svelte requires the render fn to return a single element, so we always
// wrap plain strings in a <span>.
export function slot(content) {
  if (typeof content === 'function') {
    return createRawSnippet(() => ({ render: content }));
  }
  return createRawSnippet(() => ({ render: () => `<span>${String(content)}</span>` }));
}
