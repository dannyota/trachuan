import { expect, it, vi } from 'vitest'
import { createSummaryApi } from '../app/api/summary'

it('omits credentials and exposes only public status and summary reads', async () => {
  const requests: Request[] = []
  const fetch = vi.fn(async (request: Request) => {
    requests.push(request)
    return new Response('{}', { headers: { 'content-type': 'application/json' } })
  })
  const api = createSummaryApi('https://example.invalid', fetch)
  await api.status()
  await api.summary('vn:vbpl:1')
  expect(Object.keys(api).sort()).toEqual(['status', 'summary'])
  expect(requests.map(request => new URL(request.url).pathname)).toEqual(['/api/v1/public/status', '/api/v1/public/documents/vn%3Avbpl%3A1'])
  for (const request of requests) {
    expect(request.credentials).toBe('omit')
    expect(request.headers.has('cookie')).toBe(false)
    expect(request.headers.has('authorization')).toBe(false)
  }
})
