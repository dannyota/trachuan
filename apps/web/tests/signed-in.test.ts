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
    const { default: guard } = await import('../app/middleware/signed-in.global')
    const to = { path: '/vn/search', fullPath: '/vn/search?q=x', meta: { signedIn: true } }
    expect(await guard(to as never, {} as never)).toBe('/api/v1/auth/google/start?next=%2Fvn%2Fsearch')
    expect(requested).toBe('/vn/search?q=x')
  } finally {
    vi.unstubAllGlobals()
  }
})


it('uses matched route metadata to decide whether to load a session', async () => {
  const { vi } = await import('vitest')
  const { createMemoryHistory, createRouter } = await import('vue-router')
  const routerOptions = (await import('../app/router.options')).default
  const router = createRouter({
    ...routerOptions,
    history: createMemoryHistory(),
    routes: [
      { path: '/account', component: {}, meta: { signedIn: true } },
      { path: '/vn/search', component: {}, meta: { signedIn: true } },
      { path: '/public', component: {} },
      { path: '/future', component: {}, meta: { signedIn: true } },
    ],
  })
  let loads = 0
  vi.stubGlobal('defineNuxtRouteMiddleware', (handler: unknown) => handler)
  vi.stubGlobal('useApi', () => ({ session: { status: 'unknown' }, loadSession: async () => { loads++ } }))
  try {
    const { default: guard } = await import('../app/middleware/signed-in.global')
    for (const path of ['/account', '/vn/search', '/future']) {
      await guard(router.resolve(path) as never, {} as never)
    }
    expect(loads).toBe(3)
    for (const path of ['/public', '/account/', '/Account', '/VN/search']) {
      const route = router.resolve(path)
      if (path !== '/public') expect(route.matched).toHaveLength(0)
      await guard(route as never, {} as never)
    }
    expect(loads).toBe(3)
  } finally {
    vi.unstubAllGlobals()
  }
})
