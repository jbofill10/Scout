import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'

// https://vite.dev/config/
// Vendor code is kept in two chunks: OpenTelemetry (self-contained, large,
// rarely changes) and everything else. Splitting React, MUI and Emotion into
// separate chunks created a circular chunk graph that threw "Cannot access
// 'x' before initialization" at load and left the page blank, so the
// interdependent UI libraries stay together. Pages are split per route in
// App.tsx, which is where most of the first-paint win comes from.
function vendorChunk(id: string): string | undefined {
  if (!id.includes('node_modules')) return undefined
  if (id.includes('@opentelemetry')) return 'otel'
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
