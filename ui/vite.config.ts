import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'

// https://vite.dev/config/
// Vendor code is split by library so a change to app code does not invalidate
// the cached MUI, React or OpenTelemetry bundles, and so no single chunk
// carries everything. Pages are split per route in App.tsx.
function vendorChunk(id: string): string | undefined {
  if (!id.includes('node_modules')) return undefined
  if (id.includes('@opentelemetry')) return 'otel'
  if (id.includes('@mui') || id.includes('@emotion')) return 'mui'
  if (id.includes('@tanstack')) return 'query'
  if (id.includes('react-router')) return 'router'
  if (/node_modules\/(react|react-dom|scheduler)\//.test(id)) return 'react'
  return 'vendor'
}

export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: {
        manualChunks: vendorChunk,
      },
    },
  },
  server: {
    proxy: {
      // Proxy API requests to the ingress during development
      '/api': {
        target: 'http://localhost:30300',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
