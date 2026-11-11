import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: '.',
  workers: 1,
  testMatch: '*.spec.ts',
  use: { baseURL: `http://localhost:${process.env.E2E_EDGE_PORT || '28083'}`, browserName: 'chromium' },
})
