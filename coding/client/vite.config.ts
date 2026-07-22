import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The React client is deployed independently from the Go API. During local
// development, Vite keeps requests same-origin and proxies /api to the API.
const backend = process.env.CODING_API_PROXY ?? 'http://localhost:8787'

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
    proxy: {
      '/api': {
        target: backend,
        changeOrigin: true,
      },
    },
  },
})
