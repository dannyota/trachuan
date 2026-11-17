import { execFileSync } from 'node:child_process'
import { Window } from 'happy-dom'
import { expect, it } from 'vitest'

it('serves robots and the fixed sitemap with the legal pages date', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => new Response(JSON.stringify(new URL(request.url).pathname.endsWith('/codes') ? { release: '20990101_1', codes: ['1', "a&'b", '1'] } : { release: '20990101_1' }), { headers: { 'content-type': 'application/json' } });
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const results = [];
    for (const [path, cookie] of [['/robots.txt', ''], ['/sitemap.xml', ''], ['/privacy', ''], ['/terms', ''], ['/privacy', 'lang=en'], ['/terms', 'lang=en']]) {
      const response = await app.localFetch(path, { headers: { cookie } });
      results.push({status: response.status, headers: Object.fromEntries(response.headers), body: await response.text()});
    }
    console.log(JSON.stringify(results));
  `
  const results: Array<{ status: number, headers: Record<string, string>, body: string }> = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  for (const result of results) expect(result.status).toBe(200)
  expect(results[0]!.headers['content-type']).toBe('text/plain; charset=utf-8')
  expect(results[0]!.body).toBe('User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /mcp\nDisallow: /oauth/\nSitemap: https://trachuan.vn/sitemap.xml\n')
  expect(results[1]!.headers['content-type']).toBe('application/xml; charset=utf-8')
  const document = new Window().DOMParser
  const xml = new document().parseFromString(results[1]!.body, 'application/xml')
  expect(xml.querySelector('parsererror')).toBeNull()
  expect(xml.documentElement.namespaceURI).toBe('http://www.sitemaps.org/schemas/sitemap/0.9')
  const urls = Array.from(xml.querySelectorAll('url'))
  expect(urls.map(url => url.querySelector('loc')?.textContent)).toEqual(['https://trachuan.vn/', 'https://trachuan.vn/vn/topics', 'https://trachuan.vn/connect', 'https://trachuan.vn/sources', 'https://trachuan.vn/privacy', 'https://trachuan.vn/terms', 'https://trachuan.vn/vn/topics/1', "https://trachuan.vn/vn/topics/a%26'b"])
  expect(urls.map(url => url.querySelector('lastmod')?.textContent ?? null)).toEqual([null, null, null, null, '2026-11-14', '2026-11-14', null, null])
  expect(results[1]!.body).toContain('a%26&apos;b')
  for (const result of results.slice(2, 4)) expect(result.body).toContain('14/11/2026')
  for (const result of results.slice(4)) expect(result.body).toContain('14 Nov 2026')
})


it.each(['status', 'codes', 'release', 'malformed', 'empty', 'non-string', 'extra'])('returns an uncached 503 for %s and recovers', (failure) => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    let failed = true;
    const failure = ${JSON.stringify(failure)};
    globalThis.fetch = async request => {
      const codes = new URL(request.url).pathname.endsWith('/codes');
      if (failed && ((failure === 'status' && !codes) || (failure === 'codes' && codes))) return new Response('', { status: 503 });
      const body = codes ? { release: failed && failure === 'release' ? 'other' : '20990101_1', codes: failed && failure === 'malformed' ? null : failed && failure === 'empty' ? [''] : failed && failure === 'non-string' ? [1] : ['1', '2'], ...(failed && failure === 'extra' ? { truncated: true } : {}) } : { release: '20990101_1' };
      return new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } });
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const first = await app.localFetch('/sitemap.xml');
    const firstResult = { status: first.status, cache: first.headers.get('cache-control'), etag: first.headers.get('etag'), body: await first.text() };
    failed = false;
    const second = await app.localFetch('/sitemap.xml');
    console.log(JSON.stringify({ first: firstResult, second: { status: second.status, body: await second.text() } }));
  `
  const result = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  expect(result.first.status).toBe(503)
  expect(result.first.cache).toBe('no-cache')
  expect(result.first.etag).toBeNull()
  expect(result.first.body).not.toContain('<urlset')
  expect(result.second.status).toBe(200)
  const xml = new (new Window().DOMParser)().parseFromString(result.second.body, 'application/xml')
  expect(Array.from(xml.querySelectorAll('loc')).map(loc => loc.textContent)).toEqual(['https://trachuan.vn/', 'https://trachuan.vn/vn/topics', 'https://trachuan.vn/connect', 'https://trachuan.vn/sources', 'https://trachuan.vn/privacy', 'https://trachuan.vn/terms', 'https://trachuan.vn/vn/topics/1', 'https://trachuan.vn/vn/topics/2'])
})
