import { describe, expect, it } from 'vitest'
import { createApi } from '../app/api/client'

const account = (token: string) => ({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: token })
const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } })

describe('CSRF', () => {
  it('loads a token and retries once after rejection', async () => {
    const calls: string[] = []
    const api = createApi({ origin: 'http://localhost', fetch: async (request) => {
      const req = request as Request
      calls.push(`${req.method} ${new URL(req.url).pathname} ${req.headers.get('x-csrf-token') || ''}`)
      if (req.method === 'GET') return json(account(calls.length === 1 ? 'old' : 'new'))
      return calls.length === 2 ? json({ error: { code: 'csrf_rejected', message: 'rejected' } }, 403) : new Response(null, { status: 204 })
    } })
    await api.client.DELETE('/api/v1/me', { params: { header: { 'X-CSRF-Token': '' } } })
    expect(calls).toEqual(['GET /api/v1/me ', 'DELETE /api/v1/me old', 'GET /api/v1/me ', 'DELETE /api/v1/me new'])
  })
  it('throws after a second rejection', async () => {
    let attempts = 0
    const api = createApi({ origin: 'http://localhost', fetch: async (request) => {
      if ((request as Request).method === 'GET') return json(account('token'))
      attempts++
      return json({ error: { code: 'csrf_rejected', message: 'rejected' } }, 403)
    } })
    await expect(api.client.DELETE('/api/v1/me', { params: { header: { 'X-CSRF-Token': '' } } })).rejects.toThrow()
    expect(attempts).toBe(2)
  })
})

it('navigates on a 401 from a user action', async () => {
  let redirected = false
  const api = createApi({ origin: 'http://localhost', onUnauthorized: () => { redirected = true }, fetch: async () => json({ error: { code: 'unauthenticated', message: 'sign in' } }, 401) })
  await api.client.GET('/api/v1/me')
  expect(redirected).toBe(true)
})
