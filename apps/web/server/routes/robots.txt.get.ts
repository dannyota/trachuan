export default defineEventHandler((event) => {
  setResponseHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
  const origin = useRuntimeConfig(event).public.siteOrigin.replace(/\/$/, '')
  return `User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /mcp\nDisallow: /oauth/\nSitemap: ${origin}/sitemap.xml\n`
})
