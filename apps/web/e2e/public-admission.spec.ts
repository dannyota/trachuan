import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'

test('the 121st request from a client is rejected', async ({ request }, testInfo) => {
  const headers = { 'X-Forwarded-For': clientAddress({ ...testInfo, testIndex: 0 }) }
  for (let i = 0; i < 120; i++) {
    const response = await request.get('/', { headers })
    expect(response.status()).toBe(200)
  }
  const response = await request.get('/', { headers })
  expect(response.status()).toBe(429)
  expect(Number(response.headers()['retry-after'])).toBeGreaterThan(0)
})

test('cached pages vary by normalized language only', async ({ request }, testInfo) => {
  const headers = { 'X-Forwarded-For': clientAddress({ ...testInfo, testIndex: 1 }) }
  const vi = await request.get('/sources?view=first', { headers })
  const invalid = await request.get('/sources?view=second', { headers: { ...headers, Cookie: 'lang=xx' } })
  const en = await request.get('/sources', { headers: { ...headers, Cookie: 'lang=en' } })
  expect(vi.status()).toBe(200)
  expect(invalid.status()).toBe(200)
  expect(en.status()).toBe(200)
  const viHTML = await vi.text()
  expect(await invalid.text()).toBe(viHTML)
  expect(viHTML).toContain('Nguồn dữ liệu')
  const enHTML = await en.text()
  expect(enHTML).toContain('Sources')
  expect(enHTML).not.toBe(viHTML)
})


test('public responses revalidate while private routes are not stored', async ({ request }, testInfo) => {
  const headers = { 'X-Forwarded-For': clientAddress({ ...testInfo, testIndex: 2 }) }
  for (const path of ['/llms.txt', '/', '/connect', '/sources', '/privacy', '/terms']) {
    const miss = await request.get(path, { headers })
    const hit = await request.get(path, { headers })
    expect(miss.status(), path).toBe(200)
    expect(hit.status(), path).toBe(200)
    expect(miss.headers()['cache-control'], path).toBe('no-cache')
    expect(hit.headers()['cache-control'], path).toBe('no-cache')
    expect(miss.headers().etag, path).toBeTruthy()
    expect(hit.headers().etag, path).toBe(miss.headers().etag)
  }
  for (const path of ['/connect', '/sources', '/privacy', '/terms', '/llms.txt']) {
    const alias = await request.get(`${path}/?query=ignored`, { headers, maxRedirects: 0 })
    expect(alias.status(), path).toBe(308)
    expect(alias.headers().location, path).toBe(path)
  }
  const asset = await request.get('/csp-bootstrap.js', { headers })
  expect(asset.status()).toBe(200)
  expect(asset.headers()['cache-control']).toBe('no-store')
  expect(asset.headers()['content-security-policy']).toContain("script-src 'self'")
  for (const path of ['/login', '/account', '/vn/x']) {
    const response = await request.get(path, { headers })
    expect(response.headers()['cache-control'], path).toBe('no-store')
  }
})

test('public revalidation uses ETags and preserves response headers', async ({ request }, testInfo) => {
  const headers = { 'X-Forwarded-For': clientAddress({ ...testInfo, testIndex: 3 }) }
  for (const path of ['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt']) {
    const first = await request.get(path, { headers })
    expect(first.headers()['last-modified'], path).toBeUndefined()
    const etag = first.headers().etag
    expect(etag, path).toBeTruthy()
    const mismatch = await request.get(path, {
      headers: { ...headers, 'If-None-Match': '"not-the-etag"', 'If-Modified-Since': 'Wed, 01 Jan 3000 00:00:00 GMT' },
    })
    expect(mismatch.status(), path).toBe(200)
    expect(mismatch.headers()['last-modified'], path).toBeUndefined()
    if (path === '/') expect(await mismatch.text()).toContain('Tra cứu pháp luật')
    const match = await request.get(path, { headers: { ...headers, 'If-None-Match': etag! } })
    expect(match.status(), path).toBe(304)
    expect(match.headers()['cache-control'], path).toBe('no-cache')
    expect(match.headers().vary?.toLowerCase().split(/\s*,\s*/), path).toContain('cookie')
    expect(match.headers()['last-modified'], path).toBeUndefined()
  }
})
