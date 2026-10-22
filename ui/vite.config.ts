import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': `http://localhost:${process.env.VITE_API_PORT || 8633}`,
      '/ws': {
        target: `ws://localhost:${process.env.VITE_API_PORT || 8633}`,
        ws: true,
      },
    },
  },
  build: {
    outDir: '../static',
    emptyOutDir: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
  },
})
