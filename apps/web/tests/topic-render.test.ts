import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

it('renders and caches anonymous topic pages, rejects unknown nodes and retries failures', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const calls = [];
    globalThis.fetch = async request => {
      const url = new URL(request.url);
      calls.push({ path: url.pathname, query: url.search, cookie: request.headers.get('cookie') });
      if (url.pathname.endsWith('/9')) return new Response(JSON.stringify({error:{code:'not_found',message:'missing'}}), {status:404,headers:{'content-type':'application/json'}});
      if (url.pathname.endsWith('/8')) return new Response('{}', {status:503,headers:{'content-type':'application/json'}});
      const root = {code:'1',name:'First',description:'Description',provisions:11,documents:2,children:[{code:'1.1',name:'Child',provisions:4,documents:1}]};
      const data = url.pathname.endsWith('/1') ? {node:root,path:[{code:'2',name:'Ancestor'},{code:'1',name:'First'}],documents:[{key:'vn:vbpl:1',number:'No. 1',title:'Document title',type:'law',status_class:'in_force',is_current:true}]} : {nodes:[root,{code:'2',name:'Second',provisions:3,documents:1}],truncated:false};
      return new Response(JSON.stringify(data), {headers:{'content-type':'application/json'}});
    };
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const results = [];
    for (const path of ['/vn/topics','/vn/topics','/vn/topics/1','/vn/topics/1','/vn/topics/9','/vn/topics/9','/vn/topics/8','/vn/topics/8']) {
      const res = await app.localFetch(path, {headers:{cookie:'session=private'}});
      results.push({status:res.status,headers:Object.fromEntries(res.headers),body:await res.text()});
    }
    const en = await app.localFetch('/vn/topics', {headers:{cookie:'lang=en'}});
    const nodeEn = await app.localFetch('/vn/topics/1', {headers:{cookie:'lang=en'}});
    const nodeVi = await app.localFetch('/vn/topics/1', {headers:{'if-none-match':nodeEn.headers.get('etag'),'if-modified-since':'Wed, 01 Jan 3000 00:00:00 GMT'}});
    const nodeMatch = await app.localFetch('/vn/topics/1', {headers:{'if-none-match':results[2].headers.etag}});
    const vi = await app.localFetch('/vn/topics', {headers:{'if-none-match':en.headers.get('etag'),'if-modified-since':'Wed, 01 Jan 3000 00:00:00 GMT'}});
    const matched = await app.localFetch('/vn/topics', {headers:{'if-none-match':results[0].headers.etag}});
    console.log(JSON.stringify({results,calls,en:{body:await en.text(),headers:Object.fromEntries(en.headers)},nodeEn:{body:await nodeEn.text()},nodeVi:nodeVi.status,nodeMatch:{status:nodeMatch.status,headers:Object.fromEntries(nodeMatch.headers)},vi:vi.status,matched:{status:matched.status,headers:Object.fromEntries(matched.headers)}}));
  `
  const { results, calls, en, vi, matched, nodeEn, nodeVi, nodeMatch } = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  expect(results[0].status).toBe(200)
  expect(results[0].body.indexOf('First')).toBeLessThan(results[0].body.indexOf('Second'))
  expect(results[0].body).toContain('Description')
  expect(results[0].body).toContain('11')
  expect(results[2].body).toContain('Ancestor')
  expect(results[2].body).toContain('/vn/topics/2')
  expect(results[2].body).toContain('Child')
  expect(results[2].body).toContain('Document title')
  expect(results[2].body).toContain('/vn/documents/vn:vbpl:1')
  expect(results[2].body).toContain('/vn/search?node=1')
  for (const result of results.slice(0, 4)) {
    expect(result.headers['cache-control']).toBe('no-cache')
    expect(result.headers.etag).toBeTruthy()
    expect(result.headers.vary.toLowerCase()).toContain('cookie')
    expect(result.headers['last-modified']).toBeUndefined()
  }
  expect(results.slice(4, 6).map((r: { status: number, body: string }) => [r.status, r.body])).toEqual([[404, 'Not Found\n'], [404, 'Not Found\n']])
  expect(results.slice(6).map((r: { status: number }) => r.status)).toEqual([503, 503])
  expect(results[6].body).toContain('Dịch vụ chưa sẵn sàng.')
  expect(calls.filter((call: { path: string }) => call.path.endsWith('/1'))).toHaveLength(2)
  expect(calls.filter((call: { path: string, query: string }) => call.path.endsWith('/taxonomy') && call.query === '?depth=1')).toHaveLength(2)
  expect(calls.every((call: { cookie: string | null }) => call.cookie === null)).toBe(true)
  expect(calls.filter((call: { path: string }) => call.path.endsWith('/9'))).toHaveLength(2)
  expect(calls.filter((call: { path: string }) => call.path.endsWith('/8'))).toHaveLength(2)
  expect(en.body).not.toBe(results[0].body)
  expect(vi).toBe(200)
  expect(nodeEn.body).not.toBe(results[2].body)
  expect(nodeVi).toBe(200)
  expect(nodeMatch.status).toBe(304)
  expect(nodeMatch.headers['cache-control']).toBe('no-cache')
  expect(nodeMatch.headers['last-modified']).toBeUndefined()
  expect(matched.status).toBe(304)
  expect(matched.headers['cache-control']).toBe('no-cache')
  expect(matched.headers['last-modified']).toBeUndefined()
})

it('does not cache an unavailable topic tree', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    let calls = 0;
    globalThis.fetch = async () => { calls++; return new Response('{}', {status:503,headers:{'content-type':'application/json'}}) };
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const results = [];
    for (let i = 0; i < 2; i++) { const res = await app.localFetch('/vn/topics'); results.push({status:res.status,body:await res.text()}) }
    console.log(JSON.stringify({calls,results}));
  `
  const { calls, results } = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  expect(calls).toBe(2)
  for (const result of results) {
    expect(result.status).toBe(503)
    expect(result.body).toContain('Dịch vụ chưa sẵn sàng.')
  }
})

it('does not revalidate topic errors against any ETag', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => {
      const missing = new URL(request.url).pathname.endsWith('/9');
      return new Response(JSON.stringify({error:{code:missing?'not_found':'unavailable',message:'error'}}), {status:missing?404:503,headers:{'content-type':'application/json'}});
    };
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const results = [];
    for (const path of ['/vn/topics/9', '/vn/topics/8', '/vn/topics']) {
      const first = await app.localFetch(path);
      const etag = first.headers.get('etag');
      const attempts = [first];
      for (const value of ['*', '"guessed"', ...(etag ? [etag] : [])]) attempts.push(await app.localFetch(path, {headers:{'if-none-match':value}}));
      const responses = [];
      for (const res of attempts) responses.push({status:res.status,headers:Object.fromEntries(res.headers),body:await res.text()});
      results.push({path,responses});
    }
    console.log(JSON.stringify(results));
  `
  const results = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  for (const { path, responses } of results) {
    for (const response of responses) {
      expect(response.status, path).toBe(path.endsWith('/9') ? 404 : 503)
      expect(response.headers.etag, path).toBeUndefined()
      expect(response.headers['last-modified'], path).toBeUndefined()
      expect(response.headers['cache-control'], path).toBe('no-cache')
      expect(response.headers.vary.toLowerCase(), path).toContain('cookie')
      if (path.endsWith('/9')) expect(response.body).toBe('Not Found\n')
      else expect(response.body).toContain('Dịch vụ chưa sẵn sàng.')
    }
  }
})
