import { expect, it } from 'vitest'
import { createApi } from '../app/api/client'
import { accountActions } from '../app/api/account-actions'

it('sends CSRF on account actions and signs out the current session', async () => {
  const calls: string[] = []
  const session = { account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'csrf', status: 'signed-in' as const }
  const api = createApi({ origin: 'http://localhost', session, fetch: async (input) => {
    const request = input as Request
    calls.push(`${request.method} ${new URL(request.url).pathname} ${request.headers.get('x-csrf-token')}`)
    if (request.method === 'POST' && new URL(request.url).pathname.endsWith('/tokens')) return new Response(JSON.stringify({ token: 'secret' }), { status: 201, headers: { 'content-type': 'application/json' } })
    return new Response(null, { status: 204 })
  } })
  const actions = accountActions(api.client)
  await actions.createToken('name', 90)
  await actions.revokeToken(2)
  await actions.revokeGrant(3)
  await actions.revokeSession({ id: 4, current: false })
  await actions.revokeSession({ id: 5, current: true })
  await actions.deleteAccount()
  expect(calls).toEqual([
    'POST /api/v1/me/tokens csrf', 'DELETE /api/v1/me/tokens/2 csrf',
    'DELETE /api/v1/me/grants/3 csrf', 'DELETE /api/v1/me/sessions/4 csrf',
    'POST /api/v1/auth/logout csrf', 'DELETE /api/v1/me csrf',
  ])
})
