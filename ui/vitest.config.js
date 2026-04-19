import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  test: {
    environment: 'jsdom',
    include: ['src/**/__tests__/**/*.test.js'],
    globals: false,
    setupFiles: ['src/test-setup.js'],
  },
  resolve: {
    conditions: ['browser'],
  },
})
