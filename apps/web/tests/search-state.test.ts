import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'

it('keeps search fields in local component state and prevents form navigation', () => {
  const page = readFileSync('app/pages/vn/search.vue', 'utf8')
  expect(page).toMatch(/@submit\.prevent/)
  expect(page).toMatch(/autocomplete="off"/)
  expect(page).not.toMatch(/useState[^\n]*(query|asOf|inForce)/i)
  expect(page).not.toMatch(/(?:localStorage|sessionStorage|history\.|v-html)/)
})

it('serves only the form on signed-in SSR', async () => {
  const { execFileSync } = await import('node:child_process')
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async () => new Response(JSON.stringify({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' }), { headers: { 'content-type': 'application/json' } });
    const nitro = await import('./.output/server/chunks/nitro/nitro.mjs');
    const response = await nitro.g().localFetch('/vn/search');
    console.log(await response.text());
  `
  const html = execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' })
  expect(html).toContain('autocomplete="off"')
  expect(html).not.toContain('search-results')
  expect(html).not.toContain('search-card')
})
