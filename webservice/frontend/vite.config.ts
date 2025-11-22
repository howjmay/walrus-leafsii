import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      '/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
        changeOrigin: true,
      },
      '/stream': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    }
  },
  test: {
    environment: 'node',
    globals: true,
    setupFiles: ['./src/test/setup.ts']
  }
})
