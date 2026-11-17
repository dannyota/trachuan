import createClient from 'openapi-fetch'
import type { components, paths } from '@trachuan/schema/gen/openapi'
import { legalUpdated } from '../../i18n/legal-date'

function isCodes(value: unknown): value is components['schemas']['TaxonomyCodes'] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const body = value as Record<string, unknown>
  const keys = Object.keys(body).sort()
  return keys.length === 2 && keys[0] === 'codes' && keys[1] === 'release' && typeof body.release === 'string' && Array.isArray(body.codes) && body.codes.every(code => typeof code === 'string' && code.length > 0)
}

export default defineEventHandler(async (event) => {
  try {
    const config = useRuntimeConfig(event)
    const client = createClient<paths>({ baseUrl: config.apiOrigin })
    const signal = AbortSignal.timeout(3000)
    const [status, codes] = await Promise.all([
      client.GET('/api/v1/public/status', { signal }),
      client.GET('/api/v1/public/taxonomy/codes', { signal }),
    ])
    if (!status.response.ok || !codes.response.ok || typeof status.data?.release !== 'string' || !isCodes(codes.data) || status.data.release !== codes.data.release) throw new Error('unavailable')
    const origin = config.public.siteOrigin.replace(/\/$/, '')
    const paths = ['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms', ...[...new Set(codes.data.codes)].map(code => `/vn/topics/${encodeURIComponent(code)}`)]
    const urls = paths.map((path) => {
      const loc = `${origin}${path}`.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;')
      const lastmod = path === '/privacy' || path === '/terms' ? `<lastmod>${legalUpdated}</lastmod>` : ''
      return `<url><loc>${loc}</loc>${lastmod}</url>`
    }).join('')
    setResponseHeader(event, 'Content-Type', 'application/xml; charset=utf-8')
    return `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${urls}</urlset>`
  } catch {
    event.node.res.statusCode = 503
    setResponseHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
    return 'Sitemap unavailable'
  }
})
