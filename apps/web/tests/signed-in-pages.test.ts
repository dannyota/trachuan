import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

it('protects real signed-in pages without probing public pages on SSR', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    let probes = 0;
    let publicCalls = 0;
    globalThis.fetch = async request => {
      if (new URL(request.url).pathname === '/api/v1/public/status') {
        if (request.headers.has('cookie')) throw new Error('Public status received a cookie');
        publicCalls++;
        return new Response(JSON.stringify({ release: 'test', documents: 2, provisions: 3, nodes: 1 }), { headers: { 'content-type': 'application/json' } });
      }
      if (new URL(request.url).pathname !== '/api/v1/me') throw new Error('Unexpected API request');
      probes++;
      if (request.headers.get('cookie') === 'session=ok') return new Response(JSON.stringify({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' }), { headers: { 'content-type': 'application/json' } });
      return new Response(JSON.stringify({ error: { code: 'unauthenticated', message: 'sign in' } }), { status: 401, headers: { 'content-type': 'application/json' } });
    };
    const nitro = await import('./.output/server/chunks/nitro/nitro.mjs');
    const app = nitro.g();
    const results = [];
    for (const path of ['/', '/account', '/vn/search', '/vn/documents/vn:vbpl:1']) {
      const response = await app.localFetch(path);
      results.push({ status: response.status, location: response.headers.get('location'), probes, publicCalls });
    }
    for (const path of ['/account', '/vn/search', '/vn/documents/vn:vbpl:1']) {
      const response = await app.localFetch(path, { headers: { cookie: 'session=ok' } });
      results.push({ status: response.status });
    }
    console.log(JSON.stringify(results));
  `
  const output = execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' })
  const results = JSON.parse(output)
  expect(results[0]).toMatchObject({ status: 200, probes: 0, publicCalls: 1 })
  expect(results[1]).toMatchObject({ status: 302, location: '/login?next=%2Faccount', probes: 1 })
  expect(results[2]).toMatchObject({ status: 302, location: '/login?next=%2Fvn%2Fsearch', probes: 2 })
  for (const result of results.slice(4)) expect(result.status).toBe(200)
  expect(results[3]).toMatchObject({ status: 302, location: '/login?next=%2Fvn%2Fdocuments%2Fvn%3Avbpl%3A1', probes: 3 })
})
