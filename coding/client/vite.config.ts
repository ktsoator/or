import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Electron injects the random-port sidecar URL during desktop development.
// UI-only browser tests do not need an API proxy.
const backend = process.env.CODING_API_PROXY

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: './',
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, 'src'),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: backend
      ? {
          '/api': {
            target: backend,
            changeOrigin: true,
          },
        }
      : undefined,
  },
})
