import { execFileSync } from 'node:child_process'
import { Window } from 'happy-dom'
import { expect, it } from 'vitest'
import { publicSummary, forbiddenMarkers } from '../e2e/summary-data'

function run(body: string) {
  return JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', `
    import { Server } from 'node:http';
    import { hash } from './node_modules/nitropack/dist/runtime/internal/hash.mjs';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const summary = ${JSON.stringify(publicSummary)};
    let available = true;
    let release = summary.release;
    let summaryRelease = summary.release;
    let failed = false;
    let now = Date.parse('2099-01-01T16:59:59Z');
    Date.now = () => now;
    const calls = [];
    globalThis.fetch = async request => {
      const path = decodeURIComponent(new URL(request.url).pathname);
      calls.push({path,cookie:request.headers.get('cookie'),credentials:request.credentials});
      const headers = {'content-type':'application/json'};
      if (failed) return new Response('{}', {status:503,headers});
      if (path.endsWith('/status')) return new Response(JSON.stringify({release,features:{public_summaries:available}}), {headers});
      if (path.endsWith('/codes')) return new Response(JSON.stringify({release,codes:['1']}), {headers});
      if (path.endsWith('/taxonomy/1')) return new Response(JSON.stringify({release,node:{code:'1',name:'Topic'},documents:[{key:'vn:vbpl:1',number:'1',title:'Title'}]}), {headers});
      if (path === '/api/v1/public/documents/vn:vbpl:9') return new Response('{}', {status:404,headers});
      if (path.startsWith('/api/v1/public/documents/')) return new Response(JSON.stringify({...summary, release:summaryRelease, status_date:new Date(now + 7 * 3600000).toISOString().slice(0,10)}), {headers});
      if (path === '/api/v1/public/documents') return new Response(JSON.stringify({release:summaryRelease,projection:1,total:1,items:[{key:'vn:vbpl:1',index:true}]}), {headers});
      return new Response(JSON.stringify({markers:${JSON.stringify(forbiddenMarkers)}}), {headers});
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    async function page(path, headers={}) {
      const response = await app.localFetch(path, {headers});
      return {status:response.status,headers:Object.fromEntries(response.headers),body:await response.text()};
    }
    ${body}
  `], { encoding: 'utf8', maxBuffer: 10 * 1024 * 1024, timeout: 5000 }))
}

function decodedData(html: string): string {
  const document = new Window().document
  document.write(html)
  return Buffer.from(document.getElementById('__NUXT_CSP_DATA__')!.getAttribute('content')!, 'base64').toString('utf8')
}

it('renders identical credential-independent HTML and escaped legislation without private payloads', () => {
  const result = run(`
    const pages = [];
    for (const cookie of ['', '__Host-session=valid', '__Host-session=expired', '__Host-session=forged']) pages.push(await page('/vn/documents/vn:vbpl:1/summary',{cookie}));
    pages.push(await page('/vn/documents/vn:vbpl:1/summary',{cookie:'lang=en'}));
    console.log(JSON.stringify({pages,calls}));
  `)
  for (const page of result.pages) {
    expect(page.status).toBe(200)
    expect(page.headers['cache-control']).toBe('no-cache')
    expect(page.headers.vary).toContain('Cookie')
    expect(page.headers.etag).toBeTruthy()
    const document = new Window().document
    document.write(page.body)
    expect(document.querySelectorAll('h1')).toHaveLength(1)
    expect(document.querySelector('h1')?.textContent).toBe(publicSummary.document.title)
    expect(document.querySelectorAll('link[rel="canonical"]')).toHaveLength(1)
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe('https://trachuan.vn/vn/documents/vn:vbpl:1/summary')
    const blocks = document.querySelectorAll('script[type="application/ld+json"]')
    expect(blocks).toHaveLength(1)
    expect(blocks[0]!.textContent).toContain('\\u003c')
    const graph = JSON.parse(blocks[0]!.textContent)['@graph']
    expect(graph).toHaveLength(1)
    expect(graph[0]).toMatchObject({ '@type': 'Legislation', name: publicSummary.document.title, legislationJurisdiction: 'VN', inLanguage: 'vi' })
    expect(graph[0]).not.toHaveProperty('legislationLegalForce')
    expect(graph[0]).not.toHaveProperty('status')
    const decoded = ['__NUXT_CSP_DATA__', '__NUXT_CSP_CONFIG__'].map(id => Buffer.from(document.getElementById(id)!.getAttribute('content')!, 'base64').toString('utf8'))
    for (const payload of [page.body, ...decoded]) for (const marker of forbiddenMarkers) expect(payload).not.toContain(marker)
    expect(page.body).toContain('Chương được duyệt')
    expect(page.body).toContain('Điều được duyệt')
    expect(page.body).toContain('Chủ đề mẫu')
    expect(document.querySelector('.reader-unit')).toBeNull()
  }
  for (const page of result.pages.slice(1, 4)) expect(page.body).toBe(result.pages[0].body)
  expect(result.pages[4].body).toContain('Read the full text')
  expect(result.calls.every((call: { path: string, cookie: null, credentials: string }) => /^\/api\/v1\/public\/(status|documents\/)/.test(call.path) && call.cookie === null && call.credentials === 'omit')).toBe(true)
  expect(result.calls).toHaveLength(3)
})

it.each([404, 503])('keeps a public %s error uncached even with its literal body ETag', (status) => {
  const result = run(`
    available = ${status !== 404};
    failed = ${status === 503};
    const first = await page('/vn/documents/vn:vbpl:1/summary');
    const etag = first.headers.etag ?? 'W/"' + hash(first.body) + '"';
    const repeated = await page('/vn/documents/vn:vbpl:1/summary', {'if-none-match':etag});
    console.log(JSON.stringify({first,repeated}));
  `)
  for (const page of [result.first, result.repeated]) {
    expect(page.status).toBe(status)
    expect(page.headers.etag).toBeUndefined()
    expect(page.headers['cache-control']).toBe('no-cache')
    expect(page.body).toContain(status === 503 ? 'Bạn thử lại sau.' : 'Không có trang này.')
  }
})

it('does not cache a status and summary release mismatch and recovers immediately', () => {
  const result = run(`
    release = '20990102_1';
    const mismatch = await page('/vn/documents/vn:vbpl:1/summary');
    summaryRelease = release;
    const recovered = await page('/vn/documents/vn:vbpl:1/summary');
    console.log(JSON.stringify({mismatch,recovered,calls}));
  `)
  expect(result.mismatch.status).toBe(503)
  expect(result.mismatch.headers.etag).toBeUndefined()
  expect(result.mismatch.headers['cache-control']).toBe('no-cache')
  expect(result.mismatch.body).toContain('Bạn thử lại sau.')
  expect(result.mismatch.body).not.toContain('Chương được duyệt')
  expect(result.recovered.status).toBe(200)
  expect(result.recovered.body).toContain('Chương được duyệt')
  expect(decodedData(result.recovered.body)).toContain('"20990102_1"')
  expect(result.calls).toHaveLength(4)
})

it('expires warm summaries through activation, rollback without capability and reactivation', () => {
  const result = run(`
    const initial = await page('/vn/documents/vn:vbpl:1/summary');
    const warm = await page('/vn/documents/vn:vbpl:1/summary');
    release = summaryRelease = '20990102_1';
    now += 300001;
    const activated = await page('/vn/documents/vn:vbpl:1/summary');
    await page('/vn/documents/vn:vbpl:1/summary');
    release = '20981201_1';
    available = false;
    now += 300001;
    const rollback = await page('/vn/documents/vn:vbpl:1/summary', {'if-none-match':activated.headers.etag});
    release = summaryRelease = '20990102_1';
    available = true;
    now += 30000;
    const reactivated = await page('/vn/documents/vn:vbpl:1/summary');
    const finalWarm = await page('/vn/documents/vn:vbpl:1/summary');
    console.log(JSON.stringify({initial,warm,activated,rollback,reactivated,finalWarm,calls}));
  `)
  expect(result.initial.status).toBe(200)
  expect(result.warm.body).toBe(result.initial.body)
  expect(result.activated.status).toBe(200)
  expect(decodedData(result.activated.body)).toContain('"20990102_1"')
  expect(result.activated.headers.etag).not.toBe(result.initial.headers.etag)
  expect(result.rollback.status).toBe(404)
  expect(result.rollback.headers.etag).toBeUndefined()
  expect(result.rollback.body).toContain('Không có trang này.')
  expect(result.reactivated.status).toBe(200)
  expect(result.reactivated.body).toContain('Chương được duyệt')
  expect(result.finalWarm.body).toBe(result.reactivated.body)
  expect(result.calls).toHaveLength(7)
})

it('places identity, dates and text availability in the reader band and content in its text column', () => {
  const result = run(`
    summary.document.text_complete = false;
    summary.document.effective_to = '2099-12-31';
    console.log(JSON.stringify(await page('/vn/documents/vn:vbpl:1/summary')));
  `)
  const document = new Window().document
  document.write(result.body)
  const band = document.querySelector('.reader-identity')
  expect(band).not.toBeNull()
  expect(band!.querySelector('.stamp')).not.toBeNull()
  expect(band?.querySelector('h1')?.textContent).toBe(publicSummary.document.title)
  expect(band?.querySelector('.summary-read')?.textContent).toBe('Đọc toàn văn')
  expect(band?.textContent).toContain('Nội dung văn bản chưa đầy đủ.')
  const dates = band?.querySelector('.summary-dates')
  expect(dates?.querySelectorAll('time')).toHaveLength(3)
  expect(dates?.textContent).toBe('Ban hành 01/12/2098 · Hiệu lực từ 01/01/2099 · Hết hiệu lực 31/12/2099')
  const content = document.querySelector('.reader .reader-content')
  expect(content?.textContent).toContain('Chương được duyệt')
  expect(content?.textContent).toContain('Văn bản liên quan')
  expect(content?.textContent).toContain('Chủ đề mẫu')
  expect(document.querySelector('.reader-unit')).toBeNull()
})

it('expires warm summaries at Vietnam midnight and availability changes within 300 seconds', () => {
  const result = run(`
    const first = await page('/vn/documents/vn:vbpl:1/summary');
    now += 1000;
    const midnight = await page('/vn/documents/vn:vbpl:1/summary');
    available = false;
    now += 300001;
    const off = await page('/vn/documents/vn:vbpl:1/summary',{'if-none-match':'*'});
    available = true;
    now += 30000;
    const restored = await page('/vn/documents/vn:vbpl:1/summary');
    failed = true;
    now += 300001;
    const unavailable = await page('/vn/documents/vn:vbpl:1/summary',{'if-none-match':'*'});
    console.log(JSON.stringify({first,midnight,off,restored,unavailable}));
  `)
  expect(result.first.body).toContain('01/01/2099')
  expect(result.midnight.body).toContain('02/01/2099')
  expect(result.midnight.headers.etag).not.toBe(result.first.headers.etag)
  expect(result.restored.status).toBe(200)
  for (const [name, status] of [['off', 404], ['unavailable', 503]] as const) {
    expect(result[name].status).toBe(status)
    expect(result[name].headers.etag).toBeUndefined()
    expect(result[name].headers['cache-control']).toBe('no-cache')
  }
  expect(result.off.body).toContain('Không có trang này.')
  expect(result.unavailable.body).toContain('Bạn thử lại sau.')
})

it('marks nonindex summaries and renders only text availability and fixed validity labels', () => {
  const result = run(`
    summary.index = false;
    summary.document.has_text = false;
    summary.document.status.unresolved = true;
    summary.document.gap_kinds = ['missing_effective_date'];
    const missing = await page('/vn/documents/vn:vbpl:1/summary');
    summary.document.has_text = true;
    summary.document.text_complete = false;
    const incomplete = await page('/vn/documents/vn:vbpl:2/summary');
    console.log(JSON.stringify({missing,incomplete}));
  `)
  expect(result.missing.headers['x-robots-tag']).toBe('noindex')
  expect(result.missing.body).toContain('Hiệu lực chưa xác định: các nguồn mâu thuẫn')
  expect(result.missing.body).toContain('Thông tin hiệu lực chưa đầy đủ')
  expect(result.missing.body).toContain('Tra Chuẩn chưa có nội dung văn bản này.')
  expect(result.missing.body).not.toContain('>Đọc toàn văn<')
  expect(result.incomplete.body).toContain('Nội dung văn bản chưa đầy đủ.')
  expect(result.incomplete.body).toContain('>Đọc toàn văn<')
})

it('returns identical cold-cache HTML for each presented credential', () => {
  const pages = ['', '__Host-session=valid', '__Host-session=expired', '__Host-session=forged'].map(cookie => run(`
    const result = await page('/vn/documents/vn:vbpl:1/summary', {cookie:${JSON.stringify(cookie)}});
    console.log(JSON.stringify({result,calls}));
  `))
  for (const page of pages) {
    expect(page.result.status).toBe(200)
    expect(page.result.body).toBe(pages[0].result.body)
    expect(page.calls).toHaveLength(2)
    expect(page.calls.every((call: { cookie: null }) => call.cookie === null)).toBe(true)
  }
})

it('serves oversized renders without storing them and revalidates stored pages by ETag', () => {
  const result = run(`
    const first = await page('/vn/documents/vn:vbpl:1/summary');
    const match = await page('/vn/documents/vn:vbpl:1/summary', {'if-none-match':first.headers.etag});
    const otherLanguage = await page('/vn/documents/vn:vbpl:1/summary', {cookie:'lang=en','if-none-match':first.headers.etag});
    summary.outline = Array.from({length:1500}, (_,i) => ({kind:'dieu',label:'Điều '+i,anchor:'dieu-'+i}));
    const large = await page('/vn/documents/vn:vbpl:2/summary');
    const count = calls.length;
    const again = await page('/vn/documents/vn:vbpl:2/summary');
    console.log(JSON.stringify({match,otherLanguage,large,again,count,calls}));
  `)
  expect(result.match.status).toBe(304)
  expect(result.match.headers['cache-control']).toBe('no-cache')
  expect(result.match.headers.vary).toContain('Cookie')
  expect(result.otherLanguage.status).toBe(200)
  expect(Buffer.byteLength(result.large.body)).toBeGreaterThan(256 * 1024)
  expect(result.large.status).toBe(200)
  expect(result.again.status).toBe(200)
  expect(result.calls.length).toBe(result.count + 1)
})

it('limits summary renders to two and keeps two other render slots usable', () => {
  const result = run(`
    await page('/vn/documents/vn:vbpl:1/summary');
    let unblock;
    const blocked = new Promise(resolve => {unblock=resolve});
    let ready;
    let started = 0;
    const waiting = new Promise(resolve => {ready=resolve});
    app.hooks.hook('render:before', async context => {
      if (context.response) return;
      started++;
      if (started===2) ready();
      await blocked;
    });
    const pending = ['/vn/documents/vn:vbpl:2/summary','/vn/documents/vn:vbpl:3/summary'].map(path => page(path));
    await waiting;
    const rejected = await page('/vn/documents/vn:vbpl:4/summary');
    const hit = await page('/vn/documents/vn:vbpl:1/summary');
    const others = ['/connect','/privacy'].map(path => page(path));
    while (started<4) await new Promise(resolve=>setImmediate(resolve));
    const full = await page('/terms');
    unblock();
    const completed = await Promise.all([...pending,...others]);
    console.log(JSON.stringify({rejected,hit,full,completed,started}));
  `)
  expect(result.rejected.status).toBe(429)
  expect(result.hit.status).toBe(200)
  expect(result.full.status).toBe(429)
  expect(result.completed.map((page: { status: number }) => page.status)).toEqual([200, 200, 200, 200])
  expect(result.started).toBe(4)
})

it.each(['flag off', 'rollback', 'no active release', 'available'])('changes topic and sitemap links for %s with warm caches', (state) => {
  const result = run(`
    await page('/vn/topics/1');
    await page('/sitemap.xml');
    await page('/sitemap-documents.xml');
    available = ${state === 'available'};
    failed = ${state === 'no active release'};
    release = ${JSON.stringify(state === 'rollback' ? '20981201_1' : '20990101_1')};
    now += 600001;
    const topic = await page('/vn/topics/1');
    const sitemap = await page('/sitemap.xml');
    const documents = await page('/sitemap-documents.xml');
    console.log(JSON.stringify({topic,sitemap,documents}));
  `)
  if (state === 'no active release') {
    expect(result.sitemap.status).toBe(503)
    expect(result.documents.status).toBe(503)
    expect(result.topic.body).not.toContain('/vn/documents/vn:vbpl:1/summary')
  } else {
    expect(result.topic.body.includes('href="/vn/documents/vn:vbpl:1/summary"')).toBe(state === 'available')
    expect(result.sitemap.body.includes('<sitemapindex')).toBe(state === 'available')
    expect(result.documents.status).toBe(state === 'available' ? 200 : 404)
  }
})
