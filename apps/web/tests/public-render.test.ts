import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

it('renders public coverage anonymously, caches it, and omits query and account data', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    let calls = [];
    globalThis.fetch = async request => {
      calls.push({ path: new URL(request.url).pathname, cookie: request.headers.get('cookie') });
      return new Response(JSON.stringify({ release: '20260101_1', documents: 12, provisions: 34, nodes: 2, gaps: { missing: 3 }, newest_source_date: '2026-01-01' }), { headers: { 'content-type': 'application/json' } });
    };
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const results = [];
    for (const path of ['/', '/', '/sources', '/connect', '/privacy', '/terms', '/llms.txt']) {
      const res = await app.localFetch(path + '?q=secret-query-text', { headers: { cookie: 'session=private-account' } });
      results.push({ status: res.status, headers: Object.fromEntries(res.headers), text: await res.text() });
    }
    console.log(JSON.stringify({ results, calls }));
  `
  const result = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  expect(result.calls).toEqual([{ path: '/api/v1/public/status', cookie: null }, { path: '/api/v1/public/status', cookie: null }])
  for (const page of result.results) {
    expect(page.status).toBe(200)
    expect(page.text).not.toContain('secret-query-text')
    expect(page.text).not.toContain('private-account')
    expect(page.headers['cache-control']).toContain('no-cache')
  }
  expect(result.results[2].text).toContain('01/01/2026')
  expect(result.results[2].text).toContain('3 trường hợp')
  expect(result.results[2].text).not.toContain('missing: 3')
  expect(result.results[6].headers['content-type']).toContain('text/plain')
})
