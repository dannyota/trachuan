import { describe, expect, it } from 'vitest'
import { loginUrl, safeNext } from '../app/api/next'

describe('login destination', () => {
  it('keeps only a valid document release', () => {
    expect(safeNext('/vn/search?q=x')).toBe('/vn/search')
    expect(safeNext('/vn/documents/vn:1?release=20261111_2&q=x')).toBe('/vn/documents/vn:1?release=20261111_2')
    expect(safeNext('/vn/documents/vn:1?release=bad')).toBe('/vn/documents/vn:1')
    expect(loginUrl('/account')).toBe('/api/v1/auth/google/start?next=%2Faccount')
  })
  it.each(['//host', '/\\host', '/a\\b', '/a\u0001b', '/a/../b', '/a/./b', '/%2fhost', '/a/%2e%2e/b', '/a/%5cb', '/a/%00', '/a/%zz', `/${'a'.repeat(512)}`])('rejects %s', (path) => {
    expect(safeNext(path)).toBe('/')
  })
})

it('does not include a fragment in the login destination', () => {
  expect(safeNext('/vn/search?x=y#question')).toBe('/vn/search')
  expect(safeNext('/account#question')).toBe('/account')
})

it('loads the session for protected routes and redirects a signed-out visitor', async () => {
  const { vi } = await import('vitest')
  const session = { status: 'unknown' }
  let requested = ''
  vi.stubGlobal('defineNuxtRouteMiddleware', (handler: unknown) => handler)
  vi.stubGlobal('useApi', (options: { route: { fullPath: string } }) => {
    requested = options.route.fullPath
    return { session, loadSession: async () => { session.status = 'signed-out' } }
  })
  vi.stubGlobal('navigateTo', (url: string) => url)
  try {
    const { default: guard } = await import('../app/middleware/signed-in')
    const to = { path: '/vn/search', fullPath: '/vn/search?q=x' }
    expect(await guard(to as never, {} as never)).toBe('/api/v1/auth/google/start?next=%2Fvn%2Fsearch')
    expect(requested).toBe('/vn/search?q=x')
  } finally {
    vi.unstubAllGlobals()
  }
})
