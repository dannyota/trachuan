import { execFileSync } from 'node:child_process'
import { Window } from 'happy-dom'
import { expect, it } from 'vitest'

it('serves robots and the fixed sitemap with the legal pages date', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
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
  expect(urls.map(url => url.querySelector('loc')?.textContent)).toEqual(['https://trachuan.vn/', 'https://trachuan.vn/vn/topics', 'https://trachuan.vn/connect', 'https://trachuan.vn/sources', 'https://trachuan.vn/privacy', 'https://trachuan.vn/terms'])
  expect(urls.map(url => url.querySelector('lastmod')?.textContent ?? null)).toEqual([null, null, null, null, '2026-11-14', '2026-11-14'])
  for (const result of results.slice(2, 4)) expect(result.body).toContain('14/11/2026')
  for (const result of results.slice(4)) expect(result.body).toContain('14 Nov 2026')
})
