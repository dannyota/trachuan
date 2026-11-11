import { languageFromCookie } from '../../i18n'
import { clientAddress, createAdmission } from '../utils/public-admission'
import { publicPath } from '../utils/public-path'

const admission = createAdmission(Date.now)

export default defineEventHandler((event) => {
  event.node.req.headers['x-trachuan-language'] = languageFromCookie(event.node.req.headers.cookie ?? '')
  const url = event.node.req.url ?? ''
  const path = publicPath(url)
  if (!path) return
  delete event.node.req.headers['if-modified-since']
  const peer = event.node.req.socket.remoteAddress ?? ''
  const forwarded = event.node.req.headers['x-forwarded-for']
  const ranges = useRuntimeConfig(event).trustedProxies.split(',').filter(Boolean)
  const address = clientAddress(peer, Array.isArray(forwarded) ? forwarded.join(',') : forwarded ?? '', ranges)
  if (!admission.allow(address)) {
    setResponseStatus(event, 429)
    setResponseHeader(event, 'Retry-After', 60)
    setResponseHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
    return 'Too Many Requests\n'
  }
  if (url.split('?')[0] !== path) return sendRedirect(event, path, 308)
  event.node.req.originalUrl = path
})
