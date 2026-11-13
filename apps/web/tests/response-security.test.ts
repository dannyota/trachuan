import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'
import { APP_CSP } from '../app/utils/csp'

function runServer(body: string): unknown {
  const script = `
    import { Server } from 'node:http';
    import { pathToFileURL } from 'node:url';
    import { resolve } from 'node:path';
    globalThis._importMeta_ = { url: pathToFileURL(resolve('.output/server/index.mjs')).href, env: process.env };
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const nitro = await import('./.output/server/chunks/nitro/nitro.mjs');
    const app = nitro.g();
    ${body}
  `
  return JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], {
    encoding: 'utf8', timeout: 5000,
  }))
}

it('counts public aliases and redirects them to the canonical path without a query', () => {
  const result = runServer(`
    const paths = [
      ['/connect/?x=1', '/connect'], ['/connect///?x=2', '/connect'],
      ['//connect?x=3', '/connect'], ['/vn/search/?x=4', '/vn/search'],
      ...['/sources', '/privacy', '/terms', '/llms.txt'].map(path => [path + '/?x=1', path]),
      ['//?x=1', '/'],
    ];
    const redirects = [];
    for (const [path, expected] of paths) {
      const res = await app.localFetch(path, { redirect: 'manual' });
      redirects.push({ status: res.status, location: res.headers.get('location'), expected });
    }
    for (let i = paths.length; i < 120; i++) await app.localFetch('/connect/?x=' + i, { redirect: 'manual' });
    const limited = await app.localFetch('/connect', { redirect: 'manual' });
    console.log(JSON.stringify({ redirects, limited: limited.status }));
  `) as { redirects: Array<{ status: number, location: string, expected: string }>, limited: number }
  for (const redirect of result.redirects) {
    expect(redirect.status).toBe(308)
    expect(redirect.location).toBe(redirect.expected)
  }
  expect(result.limited).toBe(429)
})

it('applies response security headers to HTML, JSON and static responses', () => {
  const result = runServer(`
    app.hooks.hook('render:before', (ctx) => {
      if (ctx.event.path.split('?')[0] === '/') throw new Error('render failed');
    });
    const results = [];
    for (const [path, accept] of [['/', 'text/html'], ['/', 'application/json'], ['/account', 'text/html'], ['/account', 'application/json'], ['/csp-bootstrap.js', '*/*']]) {
      const res = await app.localFetch(path, { headers: { accept } });
      results.push({ path, accept, status: res.status, headers: Object.fromEntries(res.headers) });
    }
    console.log(JSON.stringify(results));
  `) as Array<{ path: string, accept: string, status: number, headers: Record<string, string> }>
  for (const response of result) {
    if (response.path === '/csp-bootstrap.js') expect(response.status).toBe(200)
    else expect(response.status, `${response.path} ${response.accept}`).toBeGreaterThanOrEqual(400)
    expect(response.headers['content-security-policy'], `${response.path} ${response.accept}`).toBe(APP_CSP)
    expect(response.headers['cache-control'], `${response.path} ${response.accept}`).toBe(response.path === '/' ? 'no-cache' : 'no-store')
  }
})

it('rejects a fifth render without invoking the error renderer', () => {
  const result = runServer(`
    let started = 0;
    let errors = 0;
    let ready;
    const allStarted = new Promise(resolve => { ready = resolve });
    let release;
    const blocked = new Promise(resolve => { release = resolve });
    app.hooks.hook('render:before', async ctx => {
      if (ctx.event.path.startsWith('/__nuxt_error')) { errors++; return; }
      if (ctx.response) return;
      started++;
      if (started === 4) ready();
      await blocked;
    });
    const pending = ['/connect', '/sources', '/privacy', '/terms'].map(path => app.localFetch(path));
    await allStarted;
    const rejected = await app.localFetch('/');
    const body = await rejected.text();
    const result = { status: rejected.status, body, retry: rejected.headers.get('retry-after'), type: rejected.headers.get('content-type'), errors, started };
    release();
    await Promise.all(pending);
    result.recovered = (await app.localFetch('/')).status;
    console.log(JSON.stringify(result));
  `) as { status: number, body: string, retry: string, type: string, errors: number, started: number, recovered: number }
  expect(result).toEqual({ status: 429, body: 'Too Many Requests\n', retry: '1', type: 'text/plain; charset=utf-8', errors: 0, started: 4, recovered: 200 })
})

it('rejects an exhausted rate bucket without invoking the error renderer', () => {
  const result = runServer(`
    let errors = 0;
    app.hooks.hook('render:before', ctx => { if (ctx.event.path.startsWith('/__nuxt_error')) errors++; });
    for (let i = 0; i < 120; i++) await app.localFetch('/');
    const rejected = await app.localFetch('/');
    console.log(JSON.stringify({ status: rejected.status, body: await rejected.text(), retry: rejected.headers.get('retry-after'), type: rejected.headers.get('content-type'), errors }));
  `) as { status: number, body: string, retry: string, type: string, errors: number }
  expect(result).toEqual({ status: 429, body: 'Too Many Requests\n', retry: '60', type: 'text/plain; charset=utf-8', errors: 0 })
})

it('overrides security headers supplied to a direct writeHead', () => {
  const result = runServer(`
    app.h3App.stack.unshift({ route: '/', handler(event) {
      if (event.path !== '/direct') return;
      event.node.res.writeHead(418, { 'Content-Security-Policy': 'default-src *', 'Cache-Control': 'public' });
      event.node.res.end('direct');
    } });
    const res = await app.localFetch('/direct');
    console.log(JSON.stringify({ status: res.status, headers: Object.fromEntries(res.headers) }));
  `) as { status: number, headers: Record<string, string> }
  expect(result.status).toBe(418)
  expect(result.headers['content-security-policy']).toBe(APP_CSP)
  expect(result.headers['cache-control']).toBe('no-store')
})

it('rejects unknown and encoded paths without rendering', () => {
  const result = runServer(`
    let renders = 0;
    app.hooks.hook('render:before', () => { renders++; });
    const responses = [];
    for (const path of ['/CONNECT', '/%63onnect', '/nope', '/__nuxt_error', '/_nuxt/does-not-exist.js']) {
      const res = await app.localFetch(path, { redirect: 'manual' });
      responses.push({ status: res.status, body: await res.text(), type: res.headers.get('content-type') });
    }
    console.log(JSON.stringify({ responses, renders }));
  `) as { responses: Array<{ status: number, body: string, type: string }>, renders: number }
  expect(result.renders).toBe(0)
  for (const response of result.responses) {
    expect(response.status).toBe(404)
    expect(response.type).toBe('text/html; charset=utf-8')
    expect(response.body).toContain('Không tìm thấy trang')
  }
})

it('counts signed-in page requests against the address limit', () => {
  const result = runServer(`
    app.hooks.hook('render:before', ctx => {
      if (!ctx.response) ctx.response = { statusCode: 200, body: 'stub' };
    });
    const codes = [];
    for (let i = 0; i < 121; i++) codes.push((await app.localFetch('/account')).status);
    console.log(JSON.stringify({ first: codes.slice(0, 120), last: codes[120] }));
  `) as { first: number[], last: number }
  expect(result.first).toEqual(Array(120).fill(200))
  expect(result.last).toBe(429)
})

it('limits concurrent signed-in page and error renders', () => {
  const result = runServer(`
    let started = 0;
    let ready;
    const allStarted = new Promise(resolve => { ready = resolve });
    let release;
    const blocked = new Promise(resolve => { release = resolve });
    app.hooks.hook('render:before', async ctx => {
      if (ctx.response) return;
      started++;
      if (started === 4) ready();
      await blocked;
      ctx.response = { statusCode: 200, body: 'stub' };
    });
    const pending = Array.from({ length: 4 }, () => app.localFetch('/account'));
    await allStarted;
    const fifth = await app.localFetch('/vn/search');
    const error = await app.localFetch('/__nuxt_error', { context: { nuxt: { '~rendering-error': true } } });
    const result = { fifth: fifth.status, error: error.status, started };
    release();
    await Promise.all(pending);
    console.log(JSON.stringify(result));
  `) as { fifth: number, error: number, started: number }
  expect(result).toEqual({ fifth: 429, error: 429, started: 4 })
})

it('counts unknown page requests against the same admission bucket', () => {
  const result = runServer(`
    const codes = [];
    for (let i = 0; i < 120; i++) codes.push((await app.localFetch(i % 2 ? '/nope' : '/_nuxt/missing.js')).status);
    const rejected = await app.localFetch('/login');
    console.log(JSON.stringify({ codes, rejected: rejected.status, body: await rejected.text() }));
  `) as { codes: number[], rejected: number, body: string }
  expect(result.codes).toEqual(Array(120).fill(404))
  expect(result.rejected).toBe(429)
  expect(result.body).toBe('Too Many Requests\n')
})

it('admits a document key as one literal route segment', () => {
  const result = runServer(`
    app.hooks.hook('render:before', ctx => { if (!ctx.response) ctx.response = { statusCode: 200, body: 'stub' }; });
    const paths = ['/vn/documents/vn:vbpl:123', '/vn/documents/vn:vbpl:123/', '/vn/documents/', '/VN/documents/vn:vbpl:123', '/vn/documents/%76n:vbpl:123'];
    const responses = [];
    for (const path of paths) {
      const res = await app.localFetch(path, { redirect: 'manual' });
      responses.push({ status: res.status, location: res.headers.get('location') });
    }
    console.log(JSON.stringify(responses));
  `) as Array<{ status: number, location: string | null }>
  expect(result).toEqual([
    { status: 200, location: null },
    { status: 308, location: '/vn/documents/vn:vbpl:123' },
    { status: 404, location: null },
    { status: 404, location: null },
    { status: 404, location: null },
  ])
})


it('exempts existing static assets from page admission', () => {
  const result = runServer(`
    app.hooks.hook('render:before', ctx => { if (!ctx.response) ctx.response = { statusCode: 200, body: 'stub' }; });
    const codes = [];
    for (let i = 0; i < 121; i++) codes.push((await app.localFetch('/csp-bootstrap.js')).status);
    const page = await app.localFetch('/login');
    console.log(JSON.stringify({ codes, page: page.status }));
  `) as { codes: number[], page: number }
  expect(result.codes).toEqual(Array(121).fill(200))
  expect(result.page).toBe(200)
})

it('serves localized unknown pages without caching or rendering', () => {
  const result = runServer(`
    let renders = 0;
    app.hooks.hook('render:before', () => { renders++; });
    const responses = [];
    for (const cookie of ['', 'lang=en']) {
      const res = await app.localFetch('/missing', {headers:{cookie,'if-none-match':'*'}});
      responses.push({status:res.status, headers:Object.fromEntries(res.headers), body:await res.text()});
    }
    console.log(JSON.stringify({renders,responses}));
  `) as { renders: number, responses: Array<{ status: number, headers: Record<string, string>, body: string }> }
  expect(result.renders).toBe(0)
  for (const [index, response] of result.responses.entries()) {
    expect(response.status).toBe(404)
    expect(response.headers['content-type']).toBe('text/html; charset=utf-8')
    expect(response.headers['cache-control']).toBe('no-store')
    expect(response.headers.etag).toBeUndefined()
    expect(response.body).toContain(index ? 'Page not found' : 'Không tìm thấy trang')
  }
})
