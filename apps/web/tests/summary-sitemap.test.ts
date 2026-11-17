import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'
import { Window } from 'happy-dom'

it('indexes complete public summary lists and splits the fixed pages', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const calls = [];
    globalThis.fetch = async request => {
      const path = new URL(request.url).pathname;
      calls.push({path, cookie: request.headers.get('cookie'), credentials: request.credentials});
      const body = path.endsWith('/status') ? {release:'20990101_1',features:{public_summaries:true}} : path.endsWith('/codes') ? {release:'20990101_1',codes:['1']} : {release:'20990101_1',projection:1,total:2,items:[{key:'vn:vbpl:1',index:true},{key:'vn:vbpl:2',index:false}]};
      return new Response(JSON.stringify(body), {headers:{'content-type':'application/json'}});
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const pages = [];
    for (const path of ['/sitemap.xml','/sitemap-pages.xml','/sitemap-documents.xml']) {
      const response = await app.localFetch(path, {headers:{cookie:'__Host-session=synthetic'}});
      pages.push({status:response.status,headers:Object.fromEntries(response.headers),body:await response.text()});
    }
    console.log(JSON.stringify({pages,calls}));
  `
  const { pages, calls } = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  for (const page of pages) { expect(page.status).toBe(200); expect(page.headers['cache-control']).toBe('no-cache') }
  const parse = (body: string) => new (new Window().DOMParser)().parseFromString(body, 'application/xml')
  expect(parse(pages[0].body).documentElement.localName).toBe('sitemapindex')
  expect(Array.from(parse(pages[0].body).querySelectorAll('loc')).map(node => node.textContent)).toEqual(['https://trachuan.vn/sitemap-pages.xml', 'https://trachuan.vn/sitemap-documents.xml'])
  expect(parse(pages[1].body).querySelectorAll('url')).toHaveLength(7)
  expect(Array.from(parse(pages[2].body).querySelectorAll('loc')).map(node => node.textContent)).toEqual(['https://trachuan.vn/vn/documents/vn:vbpl:1/summary'])
  expect(parse(pages[2].body).querySelector('lastmod')).toBeNull()
  expect(calls.filter((call: { path: string }) => call.path === '/api/v1/public/documents')).toHaveLength(1)
  expect(calls.every((call: { cookie: null, credentials: string }) => call.cookie === null && call.credentials === 'omit')).toBe(true)
})

it.each(['status', 'list', 'release', 'projection', 'total', 'key', 'index', 'duplicate', 'extra'])('returns uncached 503 on documents %s failure and then recovers', (failure) => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const failure = ${JSON.stringify(failure)};
    let failed = true;
    globalThis.fetch = async request => {
      const status = new URL(request.url).pathname.endsWith('/status');
      const headers = {'content-type':'application/json'};
      if (failed && ((status && failure==='status') || (!status && failure==='list'))) return new Response('{}',{status:503,headers});
      const body = status ? {release:'20990101_1',features:{public_summaries:true}} : {release:failed && failure==='release'?'20990102_1':'20990101_1',projection:failed && failure==='projection'?2:1,total:failed && failure==='total'?2:1,items:[{key:failed && failure==='key'?'../invalid':'vn:vbpl:1',index:failed && failure==='index'?'true':true}]};
      if (failed && !status && failure==='duplicate') {body.items.push(body.items[0]);body.total=2}
      if (failed && !status && failure==='extra') body.truncated=true;
      return new Response(JSON.stringify(body),{headers});
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const pages = [];
    for (const bad of [true,false]) {
      failed = bad;
      const response = await app.localFetch('/sitemap-documents.xml',{headers:bad?{'if-none-match':'*'}:{}});
      pages.push({status:response.status,headers:Object.fromEntries(response.headers),body:await response.text()});
    }
    console.log(JSON.stringify(pages));
  `
  const pages = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  expect(pages[0].status).toBe(503)
  expect(pages[0].headers.etag).toBeUndefined()
  expect(pages[0].headers['cache-control']).toBe('no-cache')
  expect(pages[0].headers.vary).toContain('Cookie')
  expect(pages[0].headers['content-security-policy']).toContain("script-src 'self'")
  expect(pages[0].body).toBe('Sitemap unavailable')
  expect(pages[1].status).toBe(200)
  expect(pages[1].body).toContain('/vn/documents/vn:vbpl:1/summary')
})
