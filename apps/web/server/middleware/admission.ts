import { languageFromCookie } from '../../i18n'
import { clientAddress, createAdmission } from '../utils/public-admission'

const admission = createAdmission(Date.now)
const publicPaths = new Set(['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt'])

export default defineEventHandler((event) => {
  event.node.req.headers['x-trachuan-language'] = languageFromCookie(event.node.req.headers.cookie ?? '')
  const path = event.path.split('?')[0] ?? ''
  if (!publicPaths.has(path)) return
  delete event.node.req.headers['if-modified-since']
  const response = event.node.res
  const finalizeHeaders = () => {
    if (response.headersSent) return
    response.setHeader('Cache-Control', 'no-cache')
    response.removeHeader('Last-Modified')
    const vary = response.getHeader('Vary')
    const values = (Array.isArray(vary) ? vary.join(',') : String(vary ?? ''))
      .split(',').map(value => value.trim()).filter(Boolean)
    if (!values.some(value => value.toLowerCase() === 'cookie')) values.push('Cookie')
    response.setHeader('Vary', values.join(', '))
  }
  response.end = new Proxy(response.end, {
    apply(target, receiver, args) {
      finalizeHeaders()
      return Reflect.apply(target, receiver, args)
    },
  })
  response.writeHead = new Proxy(response.writeHead, {
    apply(target, receiver, args) {
      finalizeHeaders()
      return Reflect.apply(target, receiver, args)
    },
  })
  event.context.admissionResponse = response
  event.node.req.originalUrl = path
  const peer = event.node.req.socket.remoteAddress ?? ''
  const forwarded = event.node.req.headers['x-forwarded-for']
  const ranges = useRuntimeConfig(event).trustedProxies.split(',').filter(Boolean)
  const address = clientAddress(peer, Array.isArray(forwarded) ? forwarded.join(',') : forwarded ?? '', ranges)
  if (!admission.allow(address)) {
    setResponseHeader(event, 'Retry-After', 60)
    throw createError({ statusCode: 429, statusMessage: 'Too Many Requests' })
  }
})
