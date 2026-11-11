import { appendResponseHeader, sendRedirect, setResponseStatus } from 'h3'
import { createApi } from '../api/client'
import { loginUrl } from '../api/next'
import { useSessionStore } from '../stores/session'

const publicPaths = new Set(['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt'])

export function useApi(options: { userAction?: boolean, route?: { path: string, fullPath: string, meta: Record<string, unknown> } } = {}) {
  const route = options.route || useRoute()
  const event = import.meta.server ? useRequestEvent() : undefined
  const publicRoute = import.meta.server && publicPaths.has(route.path)
  const protectedRoute = route.meta.signedIn === true
  const unavailable = options.userAction || protectedRoute ? useState<boolean>('service-unavailable', () => false) : undefined
  const origin = import.meta.server ? useRuntimeConfig().apiOrigin : window.location.origin
  return createApi({
    origin,
    cookie: event && !publicRoute ? useRequestHeader('cookie') || undefined : undefined,
    publicRoute,
    development: import.meta.dev,
    session: useSessionStore(),
    onUnavailable: options.userAction || protectedRoute ? () => {
      if (unavailable) unavailable.value = true
      if (event) setResponseStatus(event, 503)
    } : undefined,
    onSetCookie: event ? cookie => appendResponseHeader(event, 'set-cookie', cookie) : undefined,
    onUnauthorized: options.userAction || protectedRoute ? async () => {
      const destination = loginUrl(route.fullPath)
      if (import.meta.client) window.location.assign(destination)
      else if (event) await sendRedirect(event, destination)
    } : undefined,
  })
}
