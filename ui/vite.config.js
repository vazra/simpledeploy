import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 8500,
    proxy: {
      '/api': { target: 'http://localhost:9500', ws: true },
      '/ws': { target: 'ws://localhost:9500', ws: true }
    }
  }
})
