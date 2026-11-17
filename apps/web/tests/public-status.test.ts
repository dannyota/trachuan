import { execFileSync } from 'node:child_process'
import { Window } from 'happy-dom'
import { expect, it } from 'vitest'
import { publicSummary } from '../e2e/summary-data'

function run(body: string) {
  return JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    let now = Date.parse('2099-01-01T00:00:00Z');
    Date.now = () => now;
    let failure = '';
    const calls = [];
    const outgoing = [];
    let statusRelease = '20990101_1';
    let available = true;
    const summary = ${JSON.stringify(publicSummary)};
    globalThis.fetch = async request => {
      const path = decodeURIComponent(new URL(request.url).pathname);
      const headers = {'content-type':'application/json'};
      outgoing.push({path,forwarded:request.headers.get('forwarded'),xff:request.headers.get('x-forwarded-for'),realIp:request.headers.get('x-real-ip')});
      if (path.endsWith('/taxonomy/404')) return new Response('{}',{status:404,headers});
      if (path.endsWith('/taxonomy/503')) return new Response('{}',{status:503,headers});
      if (path.endsWith('/status')) {
        calls.push({cookie:request.headers.get('cookie'),authorization:request.headers.get('authorization'),forwarded:request.headers.get('x-forwarded-for'),credentials:request.credentials});
        if (failure === 'throw') throw new Error('unavailable');
        if (failure === '429') return new Response('{}',{status:429,headers});
        return new Response(JSON.stringify({release:statusRelease,features:{public_summaries:available}}),{headers});
      }
      const body = path.endsWith('/taxonomy') ? {release:summary.release,nodes:[]}
        : path.endsWith('/taxonomy/1') ? {release:summary.release,node:{code:'1',name:'Topic'},documents:[]}
        : path.endsWith('/codes') ? {release:summary.release,codes:['1']}
        : path.startsWith('/api/v1/public/documents/') ? summary
        : {release:summary.release,projection:1,total:0,items:[]};
      return new Response(JSON.stringify(body),{headers});
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    async function page(path, headers={}) {
      const response = await app.localFetch(path,{headers});
      return {status:response.status,headers:Object.fromEntries(response.headers),body:await response.text()};
    }
    ${body}
  `], { encoding: 'utf8', timeout: 5000 }))
}

it('shares status across public renders until 30 seconds after request start', () => {
  const result = run(`
    const headers = {cookie:'__Host-session=synthetic',authorization:'Bearer synthetic','x-forwarded-for':'192.0.2.1'};
    const pages = [];
    for (const key of [1,2,3]) pages.push(await page('/vn/documents/vn:vbpl:'+key+'/summary',headers));
    pages.push(...await Promise.all([4,5].map(key => page('/vn/documents/vn:vbpl:'+key+'/summary',headers))));
    for (const path of ['/sitemap.xml','/vn/topics/1','/','/sources']) pages.push(await page(path,headers));
    const initial = calls.length;
    now += 29999;
    pages.push(await page('/vn/documents/vn:vbpl:6/summary'));
    const beforeExpiry = calls.length;
    now += 1;
    pages.push(await page('/vn/documents/vn:vbpl:7/summary'));
    console.log(JSON.stringify({pages,initial,beforeExpiry,calls}));
  `)
  expect(result.pages.map((page: { status: number }) => page.status)).toEqual(Array(11).fill(200))
  expect(result.initial).toBe(1)
  expect(result.beforeExpiry).toBe(1)
  expect(result.calls).toHaveLength(2)
  expect(result.calls).toEqual(Array(2).fill({ cookie: null, authorization: null, forwarded: null, credentials: 'omit' }))
})

it.each(['429', 'throw'])('does not cache expired status failures: %s', (failure) => {
  const result = run(`
    await page('/vn/documents/vn:vbpl:1/summary');
    now += 30000;
    failure = ${JSON.stringify(failure)};
    const failedSummary = await page('/vn/documents/vn:vbpl:2/summary');
    const sitemap = await page('/sitemap.xml');
    failure = '';
    const recovered = await page('/vn/documents/vn:vbpl:3/summary');
    console.log(JSON.stringify({failedSummary,sitemap,recovered,calls}));
  `)
  expect(result.failedSummary.status).toBe(503)
  expect(result.failedSummary.headers['cache-control']).toBe('no-cache')
  expect(result.sitemap.status).toBe(503)
  expect(result.recovered.status).toBe(200)
  expect(result.calls).toHaveLength(4)
})

it('refreshes status once when a new release reaches summaries and sitemaps', () => {
  const result = run(`
    await page('/vn/documents/vn:vbpl:1/summary');
    statusRelease = summary.release = '20990102_1';
    const rendered = await page('/vn/documents/vn:vbpl:2/summary');
    const pages = await page('/sitemap-pages.xml');
    const documents = await page('/sitemap-documents.xml');
    console.log(JSON.stringify({rendered,pages,documents,calls}));
  `)
  expect(result.rendered.status).toBe(200)
  const document = new Window().document
  document.write(result.rendered.body)
  const payload = document.getElementById('__NUXT_CSP_DATA__')!.getAttribute('content')!
  expect(Buffer.from(payload, 'base64').toString('utf8')).toContain('20990102_1')
  expect(result.pages.status).toBe(200)
  expect(result.documents.status).toBe(200)
  expect(result.calls).toHaveLength(2)
})

it('returns unavailable after one refresh for each persistent release mismatch', () => {
  const result = run(`
    await page('/vn/documents/vn:vbpl:1/summary');
    summary.release = '20990102_1';
    const first = await page('/vn/documents/vn:vbpl:2/summary');
    const firstCount = calls.length;
    const second = await page('/vn/documents/vn:vbpl:3/summary');
    console.log(JSON.stringify({first,second,firstCount,calls}));
  `)
  expect(result.first.status).toBe(503)
  expect(result.second.status).toBe(503)
  expect(result.firstCount).toBe(2)
  expect(result.calls).toHaveLength(3)
})

it('never forwards client address headers on server-side requests', () => {
  const result = run(`
    const headers = {cookie:'__Host-session=synthetic','x-forwarded-for':'192.0.2.1',forwarded:'for=192.0.2.1','x-real-ip':'192.0.2.1'};
    for (const path of ['/','/sources','/vn/topics','/vn/topics/1','/vn/documents/vn:vbpl:1/summary','/sitemap.xml','/sitemap-pages.xml','/sitemap-documents.xml','/vn/search?q=x']) await page(path,headers);
    console.log(JSON.stringify({outgoing}));
  `)
  const paths = result.outgoing.map((call: { path: string }) => call.path)
  expect(paths).toEqual(expect.arrayContaining(['/api/v1/public/status', '/api/v1/public/taxonomy', '/api/v1/public/taxonomy/1', '/api/v1/public/taxonomy/codes', '/api/v1/public/documents', '/api/v1/public/documents/vn:vbpl:1', '/api/v1/me']))
  expect(result.outgoing.every((call: { forwarded: null, xff: null, realIp: null }) => call.forwarded === null && call.xff === null && call.realIp === null)).toBe(true)
})

it.each(['/sitemap-pages.xml', '/sitemap-documents.xml'])('refreshes status for a release mismatch in %s', (path) => {
  const result = run(`
    await page('/vn/documents/vn:vbpl:1/summary');
    statusRelease = summary.release = '20990102_1';
    const sitemap = await page(${JSON.stringify(path)});
    console.log(JSON.stringify({sitemap,calls}));
  `)
  expect(result.sitemap.status).toBe(200)
  expect(result.calls).toHaveLength(2)
})


it.each(['/sitemap-pages.xml', '/sitemap-documents.xml'])('rejects %s when a refresh turns summaries off', (path) => {
  const result = run(`
    await page('/vn/documents/vn:vbpl:1/summary');
    statusRelease = summary.release = '20990102_1';
    available = false;
    const mismatch = await page(${JSON.stringify(path)});
    now += 30000;
    const next = await page(${JSON.stringify(path)});
    console.log(JSON.stringify({mismatch,next}));
  `)
  expect(result.mismatch.status).toBe(503)
  expect(result.mismatch.headers['cache-control']).toBe('no-cache')
  expect(result.next.status).toBe(404)
})

it('rejects the flat sitemap when a refresh turns summaries on', () => {
  const result = run(`
    available = false;
    await page('/vn/documents/vn:vbpl:1/summary');
    statusRelease = summary.release = '20990102_1';
    available = true;
    const mismatch = await page('/sitemap.xml');
    now += 30000;
    const next = await page('/sitemap.xml');
    console.log(JSON.stringify({mismatch,next}));
  `)
  expect(result.mismatch.status).toBe(503)
  expect(result.mismatch.headers['cache-control']).toBe('no-cache')
  expect(result.next.status).toBe(200)
  expect(result.next.body).toContain('<sitemapindex')
})


it('reads topic status after taxonomy', () => {
  const result = run(`
    const topic = await page('/vn/topics/1');
    console.log(JSON.stringify({topic,outgoing}));
  `)
  expect(result.topic.status).toBe(200)
  expect(result.outgoing.map((call: { path: string }) => call.path)).toEqual(['/api/v1/public/taxonomy/1', '/api/v1/public/status'])
})

it('skips status when the topic is missing', () => {
  const result = run(`
    const topic = await page('/vn/topics/404');
    console.log(JSON.stringify({topic,outgoing}));
  `)
  expect(result.topic.status).toBe(404)
  expect(result.outgoing.map((call: { path: string }) => call.path)).toEqual(['/api/v1/public/taxonomy/404'])
})


it('skips status when the topic is unavailable', () => {
  const result = run(`
    const topic = await page('/vn/topics/503');
    console.log(JSON.stringify({topic,outgoing}));
  `)
  expect(result.topic.status).toBe(503)
  expect(result.outgoing.map((call: { path: string }) => call.path)).toEqual(['/api/v1/public/taxonomy/503'])
})
