import { expect, it } from 'vitest'
import { createApi } from '../app/api/client'
import { accountActions } from '../app/api/account-actions'

const account = (id: number) => ({ id, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' })
const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } })

for (const action of ['delete', 'create', 'revoke'] as const) {
  it(`abandons ${action} after the account changes during CSRF refresh`, async () => {
    const session: { account: ReturnType<typeof account> | null, csrfToken: string | null, status: 'unknown' | 'signed-in' | 'signed-out' } = { account: account(1), csrfToken: 'old', status: 'signed-in' }
    const calls: string[] = []
    const api = createApi({ origin: 'http://localhost', session, fetch: async (input) => {
      const request = input as Request
      calls.push(`${request.method} ${new URL(request.url).pathname}`)
      if (request.method === 'GET') return json({ ...account(2), csrf_token: 'new' })
      return json({ error: { code: 'csrf_rejected', message: 'rejected' } }, 403)
    } })
    const actions = accountActions(api.client)
    const result = action === 'delete' ? await actions.deleteAccount() : action === 'create' ? await actions.createToken('test', 90) : await actions.revokeGrant(1)
    expect(result.error?.error.code).toBe('account_changed')
    expect(calls.filter(call => !call.startsWith('GET '))).toHaveLength(1)
    expect(session.account?.id).toBe(2)
  })
}

it('abandons the action when the session disappears during CSRF refresh', async () => {
  const session: { account: ReturnType<typeof account> | null, csrfToken: string | null, status: 'unknown' | 'signed-in' | 'signed-out' } = { account: account(1), csrfToken: 'old', status: 'signed-in' }
  let attempts = 0
  const api = createApi({ origin: 'http://localhost', session, fetch: async input => {
    if ((input as Request).method === 'GET') return json({ error: { code: 'unauthenticated' } }, 401)
    attempts++
    return json({ error: { code: 'csrf_rejected' } }, 403)
  } })
  const result = await accountActions(api.client).deleteAccount()
  expect(result.error?.error.code).toBe('account_changed')
  expect(attempts).toBe(1)
  expect(session).toMatchObject({ account: null, csrfToken: null, status: 'signed-out' })
})

it('does not start an action for a changed account when the CSRF token is missing', async () => {
  const session: { account: ReturnType<typeof account> | null, csrfToken: string | null, status: 'unknown' | 'signed-in' | 'signed-out' } = { account: account(1), csrfToken: null, status: 'signed-in' }
  const methods: string[] = []
  const api = createApi({ origin: 'http://localhost', session, fetch: async input => {
    methods.push((input as Request).method)
    return json({ ...account(2), csrf_token: 'new' })
  } })
  const result = await accountActions(api.client).deleteAccount()
  expect(result.error?.error.code).toBe('account_changed')
  expect(methods).toEqual(['GET'])
  expect(session.account?.id).toBe(2)
})
