import createClient from 'openapi-fetch'
import type { components, paths } from '@trachuan/schema/gen/openapi'
import type { H3Event } from 'h3'
import { legalUpdated } from '../../i18n/legal-date'
import { publicStatus } from './public-status'
import { isDocumentKey } from '../../app/utils/document-key'

function isCodes(value: unknown): value is components['schemas']['TaxonomyCodes'] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const body = value as Record<string, unknown>
  const keys = Object.keys(body).sort()
  return keys.length === 2 && keys[0] === 'codes' && keys[1] === 'release' && typeof body.release === 'string' && Array.isArray(body.codes) && body.codes.every(code => typeof code === 'string' && code.length > 0)
}

function isList(value: unknown): value is components['schemas']['PublicSummaryList'] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const body = value as Record<string, unknown>
  if (Object.keys(body).sort().join(',') !== 'items,projection,release,total' || typeof body.release !== 'string' || body.projection !== 1 || !Array.isArray(body.items) || body.total !== body.items.length || body.items.length > 10_000) return false
  let previous = ''
  return body.items.every((item: unknown) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return false
    const entry = item as Record<string, unknown>
    if (Object.keys(entry).sort().join(',') !== 'index,key' || typeof entry.key !== 'string' || !isDocumentKey(entry.key) || typeof entry.index !== 'boolean' || entry.key <= previous) return false
    previous = entry.key
    return true
  })
}

function escapeXml(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;')
}

function xml(kind: 'urlset' | 'sitemapindex', content: string): string {
  return `<?xml version="1.0" encoding="UTF-8"?><${kind} xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${content}</${kind}>`
}

export async function sitemap(event: H3Event, kind: 'index' | 'pages' | 'documents'): Promise<string> {
  try {
    const config = useRuntimeConfig(event)
    const client = createClient<paths>({ baseUrl: config.apiOrigin, credentials: 'omit' })
    const signal = AbortSignal.timeout(3000)
    let status = await publicStatus(config.apiOrigin)
    if (!status) throw new Error('unavailable')
    const origin = config.public.siteOrigin.replace(/\/$/, '')
    const available = status.features?.public_summaries === true
    setResponseHeader(event, 'Content-Type', 'application/xml; charset=utf-8')
    if (kind !== 'index' && !available) {
      setResponseStatus(event, 404)
      setResponseHeader(event, 'Cache-Control', 'no-cache')
      return ''
    }
    if (kind === 'index' && available) return xml('sitemapindex', ['/sitemap-pages.xml', '/sitemap-documents.xml'].map(path => `<sitemap><loc>${escapeXml(origin + path)}</loc></sitemap>`).join(''))
    if (kind === 'documents') {
      const requested = Date.now()
      const list = await client.GET('/api/v1/public/documents', { signal })
      if (!list.response.ok || !isList(list.data)) throw new Error('unavailable')
      if (list.data.release !== status.release) status = await publicStatus(config.apiOrigin, requested)
      if (!status || list.data.release !== status.release || (status.features?.public_summaries === true) !== available) throw new Error('unavailable')
      return xml('urlset', list.data.items.filter(item => item.index).map(item => `<url><loc>${escapeXml(`${origin}/vn/documents/${item.key}/summary`)}</loc></url>`).join(''))
    }
    const requested = Date.now()
    const codes = await client.GET('/api/v1/public/taxonomy/codes', { signal })
    if (!codes.response.ok || !isCodes(codes.data)) throw new Error('unavailable')
    if (codes.data.release !== status.release) status = await publicStatus(config.apiOrigin, requested)
    if (!status || status.release !== codes.data.release || (status.features?.public_summaries === true) !== available) throw new Error('unavailable')
    const pages = ['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms', ...[...new Set(codes.data.codes)].map(code => `/vn/topics/${encodeURIComponent(code)}`)]
    return xml('urlset', pages.map((path) => {
      const lastmod = path === '/privacy' || path === '/terms' ? `<lastmod>${legalUpdated}</lastmod>` : ''
      return `<url><loc>${escapeXml(origin + path)}</loc>${lastmod}</url>`
    }).join(''))
  } catch {
    setResponseStatus(event, 503)
    setResponseHeader(event, 'Cache-Control', 'no-cache')
    setResponseHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
    return 'Sitemap unavailable'
  }
}
