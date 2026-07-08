/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'node:path'

// Separate from vite.config.ts so the production build config stays clean.
// jsdom environment + a setup file that polyfills the browser APIs Radix UI and
// the app touch at mount, and forces fetch to fail so the api layer falls back
// to its built-in mocks (the whole UI is exercised offline, deterministically).
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
