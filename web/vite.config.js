import { defineConfig } from 'vite'

// The build goes into the Go binary (internal/web/dist). In development the
// portal's backend runs on :8080 and Vite passes it /auth and /api.
export default defineConfig({
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
  },
  server: {
    port: 5173,
    proxy: {
      '/auth': 'http://localhost:8080',
      '/api': 'http://localhost:8080',
    },
  },
})
