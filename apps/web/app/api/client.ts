import createClient from 'openapi-fetch'
import type { components, paths } from '@trachuan/schema/gen/openapi'

type Account = components['schemas']['Account']
type Session = { account: Omit<Account, 'csrf_token'> | null, csrfToken: string | null, status: 'unknown' | 'signed-in' | 'signed-out' }
type Options = {
  origin: string
  cookie?: string
  publicRoute?: boolean
  development?: boolean
  fetch?: typeof globalThis.fetch
  onSetCookie?: (cookie: string) => void
  onUnauthorized?: () => void | Promise<void>
  onUnavailable?: () => void
  session?: Session
}

export class ApiError extends Error {
  constructor(public code: string, public status: number) { super(code) }
}

export function createApi(options: Options) {
  const session: Session = options.session || { account: null, csrfToken: null, status: 'unknown' }
  const transport = options.fetch || globalThis.fetch
  async function send(request: Request): Promise<Response> {
    const headers = new Headers(request.headers)
    headers.delete('cookie')
    if (options.cookie && !options.publicRoute) headers.set('cookie', options.cookie)
    let response: Response
    try {
      response = await transport(new Request(request, { headers }))
      if (response.status >= 500) throw new Error('unavailable')
      if (response.status !== 401 && response.status !== 204 && response.status !== 205) {
        if (!response.headers.get('content-type')?.toLowerCase().includes('application/json')) throw new Error('unavailable')
        await response.clone().json()
      }
    } catch {
      options.onUnavailable?.()
      throw new ApiError('unavailable', 503)
    }
    if (!options.publicRoute && options.onSetCookie) {
      for (const cookie of response.headers.getSetCookie()) options.onSetCookie(cookie)
    }
    return response
  }
  async function loadSession(): Promise<void> {
    if (options.publicRoute) {
      if (options.development) throw new Error('Session lookup on cached public SSR')
      return
    }
    const response = await send(new Request(new URL('/api/v1/me', options.origin)))
    if (response.status === 401) {
      session.account = null
      session.csrfToken = null
      session.status = 'signed-out'
      return
    }
    if (!response.ok) throw new ApiError('session_failed', response.status)
    const { csrf_token, ...account } = await response.json() as Account
    session.account = account
    session.csrfToken = csrf_token
    session.status = 'signed-in'
  }
  function accountChanged(): Response {
    return new Response(JSON.stringify({ error: { code: 'account_changed', message: 'Account changed' } }), { status: 409, headers: { 'content-type': 'application/json' } })
  }
  async function handle(input: Request): Promise<Response> {
    if (options.publicRoute && new URL(input.url).pathname === '/api/v1/me') {
      if (options.development) throw new Error('Session lookup on cached public SSR')
      return new Response(JSON.stringify({ error: { code: 'unauthenticated', message: 'Session lookup unavailable' } }), { status: 401, headers: { 'content-type': 'application/json' } })
    }
    const unsafe = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(input.method)
    const startedAccount = session.account?.id
    if (unsafe && !session.csrfToken) {
      await loadSession()
      if (startedAccount !== undefined && startedAccount !== session.account?.id) return accountChanged()
    }
    if (unsafe && !session.csrfToken) {
      await options.onUnauthorized?.()
      throw new ApiError('unauthenticated', 401)
    }
    const accountId = session.account?.id
    const makeRequest = () => {
      const headers = new Headers(input.headers)
      if (unsafe && session.csrfToken) headers.set('X-CSRF-Token', session.csrfToken)
      return new Request(input.clone(), { headers })
    }
    let response = await send(makeRequest())
    if (unsafe && response.status === 403) {
      const error = await response.clone().json().catch(() => null)
      if (error?.error?.code === 'csrf_rejected') {
        await loadSession()
        if (accountId === undefined || accountId !== session.account?.id || !session.csrfToken) return accountChanged()
        response = await send(makeRequest())
        if (response.status === 403 && (await response.clone().json().catch(() => null))?.error?.code === 'csrf_rejected') throw new ApiError('csrf_rejected', 403)
      }
    }
    if (response.status === 401 && !input.signal.aborted) await options.onUnauthorized?.()
    return response
  }
  const client = createClient<paths>({ baseUrl: options.origin, fetch: async (input) => {
    try {
      return await handle(input as Request)
    } catch (error) {
      if (!(error instanceof ApiError) || error.code !== 'unavailable') throw error
      return new Response(JSON.stringify({ error: { code: 'unavailable', message: 'unavailable' } }), {
        status: 503, headers: { 'content-type': 'application/json' },
      })
    }
  } })
  return { client, loadSession, session }
}
