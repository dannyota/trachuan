import { loadNuxtConfig } from '@nuxt/kit'
import { describe, expect, it } from 'vitest'

describe('route rules', () => {
  it('limits cache rules to public paths and marks private paths no-store', async () => {
    const config = await loadNuxtConfig({ cwd: process.cwd() })
    const rules = config.routeRules!
    for (const path of ['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt']) {
      expect(rules[path]?.cache).toMatchObject({ maxAge: 600, varies: ['x-trachuan-language'] })
    }
    for (const path of ['/login', '/account', '/vn/**']) {
      expect(rules[path]?.cache).toBeUndefined()
      expect(rules[path]?.headers?.['Cache-Control']).toBe('no-store')
    }
  })
})

it('sends revalidation headers on cached public responses', async () => {
  const { execFileSync } = await import('node:child_process')
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const nitro = await import('./.output/server/chunks/nitro/nitro.mjs');
    const app = nitro.g();
    const results = [];
    for (const path of ['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt']) {
      for (let i = 0; i < 2; i++) {
        const response = await app.localFetch(path);
        results.push({ path, cache: response.headers.get('cache-control'), etag: response.headers.get('etag') });
      }
    }
    const english = await app.localFetch('/', { headers: { cookie: 'lang=en' } });
    const vietnamese = await app.localFetch('/', { headers: { 'if-none-match': english.headers.get('etag') } });
    results.push({ path: 'language revalidation', cache: vietnamese.headers.get('cache-control'), status: vietnamese.status });
    for (const path of ['/login', '/account', '/vn/x']) {
      const response = await app.localFetch(path);
      results.push({ path, cache: response.headers.get('cache-control') });
    }
    console.log(JSON.stringify(results));
  `
  const output = execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' })
  const results: Array<{ path: string, cache: string, etag?: string, status?: number }> = JSON.parse(output)
  for (const result of results.slice(0, 12)) {
    expect(result.cache, result.path).toBe('no-cache')
    expect(result.etag, result.path).toBeTruthy()
  }
  expect(results[12]).toMatchObject({ cache: 'no-cache', status: 200 })
  for (const result of results.slice(13)) expect(result.cache, result.path).toBe('no-store')
})


it('revalidates public routes only by ETag', async () => {
  const { execFileSync } = await import('node:child_process')
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    const nitro = await import('./.output/server/chunks/nitro/nitro.mjs');
    const app = nitro.g();
    const results = [];
    for (const path of ['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt']) {
      const first = await app.localFetch(path);
      const etag = first.headers.get('etag');
      const mismatch = await app.localFetch(path, { headers: {
        'if-none-match': '"not-the-etag"', 'if-modified-since': 'Wed, 01 Jan 3000 00:00:00 GMT'
      } });
      const match = await app.localFetch(path, { headers: { 'if-none-match': etag } });
      results.push({ path, first: Object.fromEntries(first.headers), mismatch: {
        status: mismatch.status, headers: Object.fromEntries(mismatch.headers)
      }, match: { status: match.status, headers: Object.fromEntries(match.headers) } });
    }
    console.log(JSON.stringify(results));
  `
  const result = execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' })
  const responses: Array<{ path: string, first: Record<string, string>, mismatch: { status: number, headers: Record<string, string> }, match: { status: number, headers: Record<string, string> } }> = JSON.parse(result)
  for (const response of responses) {
    expect(response.mismatch.status, response.path).toBe(200)
    expect(response.match.status, response.path).toBe(304)
    for (const headers of [response.first, response.mismatch.headers, response.match.headers]) {
      expect(headers['cache-control'], response.path).toBe('no-cache')
      expect(headers.vary?.toLowerCase().split(/\s*,\s*/), response.path).toContain('cookie')
      expect(headers['last-modified'], response.path).toBeUndefined()
    }
  }
})

it('uses case-sensitive strict matching for client navigation', async () => {
  const { createMemoryHistory, createRouter } = await import('vue-router')
  const options = (await import('../app/router.options')).default
  const router = createRouter({
    ...options,
    history: createMemoryHistory(),
    routes: [{ path: '/connect', component: {} }],
  })
  expect(router.resolve('/connect').matched).toHaveLength(1)
  expect(router.resolve('/CONNECT').matched).toHaveLength(0)
  expect(router.resolve('/connect/').matched).toHaveLength(0)
})
