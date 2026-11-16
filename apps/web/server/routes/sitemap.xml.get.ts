import { legalUpdated } from '../../i18n/legal-date'

export default defineEventHandler((event) => {
  setResponseHeader(event, 'Content-Type', 'application/xml; charset=utf-8')
  const origin = useRuntimeConfig(event).public.siteOrigin.replace(/\/$/, '')
  const urls = ['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms'].map(path => {
    const loc = `${origin}${path}`.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;')
    const lastmod = path === '/privacy' || path === '/terms' ? `<lastmod>${legalUpdated}</lastmod>` : ''
    return `<url><loc>${loc}</loc>${lastmod}</url>`
  }).join('')
  return `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${urls}</urlset>`
})
