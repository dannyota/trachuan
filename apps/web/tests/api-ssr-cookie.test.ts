import { expect, it } from 'vitest'
import { createApi } from '../app/api/client'

it('forwards only the cookie and appends renewal cookies on private SSR', async () => {
  const received: Headers[] = []
  const cookies: string[] = []
  const api = createApi({ origin: 'http://localhost', cookie: 'session=abc', onSetCookie: value => cookies.push(value), fetch: async (request) => {
    received.push((request as Request).headers)
    return new Response(JSON.stringify({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 't' }), { headers: { 'content-type': 'application/json', 'set-cookie': 'session=renewed; Path=/' } })
  } })
  await api.loadSession()
  expect(received[0]?.get('cookie')).toBe('session=abc')
  expect(received[0]?.get('authorization')).toBeNull()
  expect(received[0]?.get('x-forwarded-for')).toBeNull()
  expect(received[0]?.get('accept-language')).toBeNull()
  expect(cookies).toEqual(['session=renewed; Path=/'])
})

it('never sends cookies from public SSR', async () => {
  let cookie: string | null = 'unread'
  const api = createApi({ origin: 'http://localhost', cookie: 'session=abc', publicRoute: true, fetch: async (request) => {
    cookie = (request as Request).headers.get('cookie')
    return new Response('{}', { headers: { 'content-type': 'application/json' } })
  } })
  await api.client.GET('/api/v1/public/status')
  expect(cookie).toBeNull()
})

it('rejects a session lookup on cached public SSR in development', async () => {
  const api = createApi({ origin: 'http://localhost', cookie: 'session=abc', publicRoute: true, development: true })
  await expect(api.loadSession()).rejects.toThrow('Session lookup on cached public SSR')
})

it('does not look up a session on cached public SSR in production', async () => {
  let called = false
  const api = createApi({ origin: 'http://localhost', publicRoute: true, fetch: async () => { called = true; return new Response('{}') } })
  await api.loadSession()
  expect(called).toBe(false)
})

it('blocks direct session requests on cached public SSR', async () => {
  let called = false
  const api = createApi({ origin: 'http://localhost', publicRoute: true, fetch: async () => { called = true; return new Response('{}') } })
  await api.client.GET('/api/v1/me')
  expect(called).toBe(false)
})
