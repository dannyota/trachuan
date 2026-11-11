import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.ts',
  use: { baseURL: 'http://127.0.0.1:3031', browserName: 'chromium' },
  webServer: [{
    command: 'node e2e/account-api-stub.mjs',
    cwd: new URL('..', import.meta.url).pathname,
    url: 'http://127.0.0.1:3032/ready',
    reuseExistingServer: false,
  }, {
    command: 'node .output/server/index.mjs',
    cwd: new URL('..', import.meta.url).pathname,
    env: { NITRO_HOST: '127.0.0.1', NITRO_PORT: '3031', NUXT_TRUSTED_PROXIES: '127.0.0.1/32', NUXT_API_ORIGIN: 'http://127.0.0.1:3032' },
    url: 'http://127.0.0.1:3031',
    reuseExistingServer: false,
  }],
})
