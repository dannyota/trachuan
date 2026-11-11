import { spawnSync } from 'node:child_process'
import { expect, it } from 'vitest'

it('renders a fixed 503 without logging an unhandled error when session transport fails', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    process.env.NUXT_API_ORIGIN = 'http://127.0.0.1:1';
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const pages = [];
    for (const path of ['/account', '/vn/search']) {
      const res = await app.localFetch(path);
      pages.push({ status: res.status, cache: res.headers.get('cache-control'), body: await res.text() });
    }
    console.log(JSON.stringify(pages));
  `
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8', timeout: 5000 })
  expect(result.status).toBe(0)
  expect(result.stderr).not.toContain('[unhandled]')
  for (const page of JSON.parse(result.stdout)) {
    expect(page.status).toBe(503)
    expect(page.cache).toBe('no-store')
    expect(page.body).toContain('Dịch vụ chưa sẵn sàng.')
    expect(page.body).not.toContain('fetch failed')
  }
})
