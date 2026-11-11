import { APP_CSP } from '../../app/utils/csp'
import { createAdmission } from '../utils/public-admission'

const publicPaths = new Set(['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt'])

export default defineNitroPlugin((nitro) => {
  const admission = createAdmission(Date.now)
  const renders = new WeakMap<object, () => void>()
  nitro.hooks.hook('render:before', (context) => {
    if (!publicPaths.has(context.event.path.split('?')[0] ?? '')) return
    const release = admission.render()
    if (!release) {
      context.event.context.admissionResponse?.setHeader('Retry-After', '1')
      throw createError({ statusCode: 429, statusMessage: 'Too Many Requests' })
    }
    renders.set(context.event.context, release)
  })
  nitro.hooks.hook('render:response', (_response, context) => {
    renders.get(context.event.context)?.()
    renders.delete(context.event.context)
  })
  nitro.hooks.hook('error', (_error, context) => {
    if (context.event) {
      renders.get(context.event.context)?.()
      renders.delete(context.event.context)
    }
  })
  nitro.hooks.hook('beforeResponse', (event) => {
    setResponseHeader(event, 'Content-Security-Policy', APP_CSP)
    const path = event.path.split('?')[0] ?? ''
    if (publicPaths.has(path)) {
      setResponseHeader(event, 'Cache-Control', 'no-cache')
    } else if (path === '/__nuxt_error') {
      const original = getQuery(event).url
      if (typeof original === 'string' && (original.split('?')[0] === '/account' || original.startsWith('/vn/'))) {
        setResponseHeader(event, 'Cache-Control', 'no-store')
      }
    }
  })
})
