import createClient from 'openapi-fetch'
import type { components, paths } from '@trachuan/schema/gen/openapi'

type Account = components['schemas']['Account']
export type Action = { accountId: number | null, csrfToken: string | null, bound: boolean }
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
      if (request.signal.aborted) throw new DOMException('Aborted', 'AbortError')
      options.onUnavailable?.()
      throw new ApiError('unavailable', 503)
    }
    if (!options.publicRoute && options.onSetCookie) {
      for (const cookie of response.headers.getSetCookie()) options.onSetCookie(cookie)
    }
    return response
  }
  async function loadSession(signal?: AbortSignal): Promise<void> {
    if (options.publicRoute) {
      if (options.development) throw new Error('Session lookup on cached public SSR')
      return
    }
    signal?.throwIfAborted()
    const response = await send(new Request(new URL('/api/v1/me', options.origin), { signal }))
    signal?.throwIfAborted()
    if (response.status === 204 || response.status === 401) {
      session.account = null
      session.csrfToken = null
      session.status = 'signed-out'
      return
    }
    if (!response.ok) throw new ApiError('session_failed', response.status)
    const { csrf_token, ...account } = await response.json() as Account
    signal?.throwIfAborted()
    session.account = account
    session.csrfToken = csrf_token
    session.status = 'signed-in'
  }
  function accountChanged(): Response {
    return new Response(JSON.stringify({ error: { code: 'account_changed', message: 'Account changed' } }), { status: 409, headers: { 'content-type': 'application/json' } })
  }
  function captureAction(): Action {
    return { accountId: session.account?.id ?? null, csrfToken: session.csrfToken, bound: false }
  }
  async function handle(input: Request, action: Action): Promise<Response> {
    if (options.publicRoute && new URL(input.url).pathname === '/api/v1/me') {
      if (options.development) throw new Error('Session lookup on cached public SSR')
      return new Response(JSON.stringify({ error: { code: 'unauthenticated', message: 'Session lookup unavailable' } }), { status: 401, headers: { 'content-type': 'application/json' } })
    }
    input.signal.throwIfAborted()
    const unsafe = !['GET', 'HEAD', 'OPTIONS'].includes(input.method)
    let { accountId, csrfToken } = action
    if (unsafe && ((action.bound && accountId === null) || accountId !== (session.account?.id ?? null))) return accountChanged()
    if (unsafe && !csrfToken) {
      await loadSession(input.signal)
      input.signal.throwIfAborted()
      if (accountId !== null && accountId !== session.account?.id) return accountChanged()
      accountId = session.account?.id ?? null
      csrfToken = session.csrfToken
    }
    if (unsafe && !csrfToken) {
      await options.onUnauthorized?.()
      throw new ApiError('unauthenticated', 401)
    }
    const makeRequest = (token: string | null) => {
      const headers = new Headers(input.headers)
      if (unsafe && token) headers.set('X-CSRF-Token', token)
      return new Request(input.clone(), { headers })
    }
    let response = await send(makeRequest(csrfToken))
    if (unsafe) input.signal.throwIfAborted()
    if (unsafe && response.status === 403) {
      const error = await response.clone().json().catch(() => null)
      if (error?.error?.code === 'csrf_rejected') {
        input.signal.throwIfAborted()
        await loadSession(input.signal)
        input.signal.throwIfAborted()
        if (accountId === null || accountId !== session.account?.id || !session.csrfToken) return accountChanged()
        csrfToken = session.csrfToken
        response = await send(makeRequest(csrfToken))
        input.signal.throwIfAborted()
        if (response.status === 403 && (await response.clone().json().catch(() => null))?.error?.code === 'csrf_rejected') throw new ApiError('csrf_rejected', 403)
      }
    }
    if (response.status === 401 && !input.signal.aborted) await options.onUnauthorized?.()
    return response
  }
  function createActionClient(action: Action) {
    return createClient<paths>({ baseUrl: options.origin, fetch: async (input) => {
      try {
        return await handle(input as Request, action)
      } catch (error) {
        if (!(error instanceof ApiError) || error.code !== 'unavailable') throw error
        return new Response(JSON.stringify({ error: { code: 'unavailable', message: 'unavailable' } }), {
          status: 503, headers: { 'content-type': 'application/json' },
        })
      }
    } })
  }
  const base = createActionClient(captureAction())
  const client = new Proxy(base, {
    get(target, property, receiver) {
      if (typeof property === 'string' && ['POST', 'PUT', 'PATCH', 'DELETE', 'TRACE', 'request'].includes(property)) {
        return (...args: unknown[]) => {
          const scoped = createActionClient(captureAction())
          return Reflect.apply(Reflect.get(scoped, property), scoped, args)
        }
      }
      return Reflect.get(target, property, receiver)
    },
  })
  return { client, captureAction, forAction: (action: Action) => createActionClient({ ...action, bound: true }), loadSession, session }
}
