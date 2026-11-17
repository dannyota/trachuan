import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

function render(scriptMode: string) {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const mode = process.argv[1];
    const calls = [];
    globalThis.fetch = async request => {
      const url = new URL(request.url);
      calls.push({ path: url.pathname, cookie: request.headers.get('cookie') });
      if (url.pathname === '/api/v1/me') return mode === 'signed-out'
        ? new Response(null, { status: 204 })
        : new Response(JSON.stringify({ id: 1, email: 'synthetic@example.invalid', csrf_token: 'synthetic' }), { headers: { 'content-type': 'application/json' } });
      if (mode === 'available') {
        const version = { code: 'fw-alpha', version: '2.1', name: 'Synthetic fw-alpha', publisher: 'Synthetic publisher', access: 'licensed', readable: false };
        const data = url.pathname.endsWith('/gaps') ? { release: '20990101', gaps: [] } : { release: '20990101', items: [{ complete: true, units: [{ kind: 'head', version }] }] };
        return new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' } });
      }
      return new Response(JSON.stringify({ error: { code: 'not_found', message: 'missing' } }), { status: 404, headers: { 'content-type': 'application/json' } });
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const paths = mode === 'available' ? ['/frameworks/fw-alpha/2.1'] : mode === 'signed-out' ? ['/frameworks/fw-alpha/2.1/control?citation=A+%281%29%2F%23%25', '/frameworks/fw-alpha/2.1/control?citation=A%5C1', '/frameworks/' + 'a'.repeat(512) + '/2.1/control?citation=A'] : ['/frameworks', '/frameworks/search', '/frameworks/fw-alpha/2.1', '/frameworks/fw-alpha/2.1/control?citation=A'];
    const results = [];
    for (const path of paths) {
      const response = await app.localFetch(path, { redirect: 'manual', headers: { cookie: 'synthetic=1' } });
      const body = await response.text();
      results.push({ status: response.status, location: response.headers.get('location'), fixed404: body.includes('Không có trang này.'), name: body.includes('Synthetic fw-alpha') });
    }
    console.log(JSON.stringify({ results, calls }));
  `
  return JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script, scriptMode], { encoding: 'utf8' })) as { results: { status: number, location: string | null, fixed404: boolean, name: boolean }[], calls: { path: string, cookie: string | null }[] }
}

it('preserves a valid encoded citation in the SSR sign-in redirect', () => {
  const [result] = render('signed-out').results
  expect(result?.status).toBe(302)
  const next = new URL(result!.location!, 'http://localhost').searchParams.get('next')
  expect(next).toBe('/frameworks/fw-alpha/2.1/control?citation=A+%281%29%2F%23%25')
  expect(new URL(next!, 'http://localhost').searchParams.get('citation')).toBe('A (1)/#%')
})

it('returns the fixed HTTP 404 for every framework page when the API returns 404', () => {
  const { results } = render('signed-in')
  expect(results).toHaveLength(4)
  for (const result of results) expect(result).toMatchObject({ status: 404, fixed404: true })
})

it('loads a version and its gaps during SSR with the incoming session cookie', () => {
  const { results, calls } = render('available')
  expect(results[0]).toMatchObject({ status: 200, name: true })
  expect(calls.map(call => call.path)).toContain('/api/v1/frameworks/gaps')
  expect(calls.some(call => call.path.startsWith('/api/v1/frameworks/versions/'))).toBe(true)
  expect(calls.every(call => call.cookie === 'synthetic=1')).toBe(true)
})

it('uses the version and index fallbacks for unsafe sign-in destinations', () => {
  const { results } = render('signed-out')
  const next = results.map(result => new URL(result.location!, 'http://localhost').searchParams.get('next'))
  expect(next[1]).toBe('/frameworks/fw-alpha/2.1')
  expect(next[2]).toBe('/frameworks')
})
