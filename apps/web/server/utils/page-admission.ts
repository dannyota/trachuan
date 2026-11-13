import type { H3Event } from 'h3'
import { getAsset } from '#nitro-internal-virtual/public-assets'
import { languageFromCookie } from '../../i18n'
import { clientAddress, createAdmission } from './public-admission'
import { matchPage } from './page-route'
import { notFoundDocument } from './not-found'

const admission = createAdmission(Date.now)

function isStaticAsset(event: H3Event, url: string): boolean {
  if (event.method !== 'GET' && event.method !== 'HEAD') return false
  const raw = url.split('?')[0] ?? ''
  let path: string
  try {
    path = decodeURIComponent(raw.replace(/%25/g, '%2525').replace(/%2f/gi, '%252F'))
      .split('?')[0] ?? ''
    path = decodeURIComponent(path.replace(/%2f/gi, '%252F'))
  } catch {
    return false
  }
  path = path.replace(/\/$/, '') || '/'
  const encodings = String(event.node.req.headers['accept-encoding'] ?? '').split(',')
    .map(value => ({ br: '.br', gzip: '.gz' }[value.trim()] ?? '')).filter(Boolean)
  for (const encoding of [...encodings, '']) {
    if (getAsset(path + encoding) || getAsset(`${path === '/' ? '' : path}/index.html${encoding}`)) return true
  }
  return false
}

export function admitPage(event: H3Event): void {
  const request = event.node.req as typeof event.node.req & { __unenv__?: { nuxt?: { '~rendering-error'?: boolean } } }
  const url = request.url ?? ''
  request.headers['x-trachuan-language'] = languageFromCookie(request.headers.cookie ?? '')
  if (url.split('?')[0] === '/__nuxt_error' && request.__unenv__?.nuxt?.['~rendering-error']) return
  if (import.meta.dev && url.startsWith('/api/')) return
  if (isStaticAsset(event, url)) return
  const page = matchPage(url)
  const peer = request.socket.remoteAddress ?? ''
  const forwarded = request.headers['x-forwarded-for']
  const ranges = useRuntimeConfig(event).trustedProxies.split(',').filter(Boolean)
  const address = clientAddress(peer, Array.isArray(forwarded) ? forwarded.join(',') : forwarded ?? '', ranges)
  if (!admission.allow(address)) {
    setResponseStatus(event, 429)
    setResponseHeader(event, 'Retry-After', 60)
    setResponseHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
    event.node.res.end('Too Many Requests\n')
    return
  }
  if (!page) {
    setResponseStatus(event, 404)
    setResponseHeader(event, 'Content-Type', 'text/html; charset=utf-8')
    event.node.res.removeHeader('ETag')
    event.node.res.end(notFoundDocument(request.headers['x-trachuan-language'] === 'en' ? 'en' : 'vi'))
    return
  }
  if (page.alias) {
    setResponseStatus(event, 308)
    setResponseHeader(event, 'Location', page.path)
    event.node.res.end()
    return
  }
  if (page.public) {
    delete request.headers['if-modified-since']
    request.originalUrl = page.path
  }
}
