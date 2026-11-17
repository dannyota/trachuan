import { expect, test } from '@playwright/test'
import { withPublicSite } from './public-fixture'

for (const available of [true, false]) {
  test(`public discovery pages with API ${available ? 'available' : 'unavailable'}`, async ({ browser }) => {
    await withPublicSite(available, async (origin) => {
      const context = await browser.newContext()
      const page = await context.newPage()
      await page.addInitScript(() => {
        window.addEventListener('securitypolicyviolation', () => { document.documentElement.dataset.cspViolation = 'true' })
      })
      const paths = ['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms']
      if (available) paths.push('/vn/topics/1')
      for (const path of paths) {
        const response = await page.goto(`${origin}${path}`)
        expect(response?.status(), path).toBe(!available && path === '/vn/topics' ? 503 : 200)
        await expect(page.locator('link[rel="canonical"]')).toHaveCount(1)
        await expect(page.locator('h1')).toHaveCount(1)
        expect((await page.title()).normalize('NFC').length).toBeLessThanOrEqual(60)
        expect((await page.locator('meta[name="description"]').getAttribute('content'))!.normalize('NFC').length).toBeLessThanOrEqual(160)
        const expectedTypes = path === '/' ? ['WebSite', 'Organization', 'WebApplication', 'Person'] : path === '/vn/topics/1' ? ['BreadcrumbList', 'ItemList'] : path === '/sources' && available ? ['Dataset'] : []
        const blocks = page.locator('script[type="application/ld+json"]')
        await expect(blocks).toHaveCount(expectedTypes.length ? 1 : 0)
        if (expectedTypes.length) {
          const graph = JSON.parse((await blocks.textContent())!)
          expect(graph['@graph'].map((node: { '@type': string }) => node['@type'])).toEqual(expectedTypes)
        }
        await expect(page.locator('html')).not.toHaveAttribute('data-csp-violation', 'true')
      }
      expect((await context.request.get(`${origin}/robots.txt`)).status()).toBe(200)
      const sitemap = await context.request.get(`${origin}/sitemap.xml`)
      expect(sitemap.status()).toBe(available ? 200 : 503)
      if (available) {
        const xml = await sitemap.text()
        expect([...xml.matchAll(/<loc>(.*?)<\/loc>/g)].map(match => match[1])).toEqual(['https://trachuan.vn/', 'https://trachuan.vn/vn/topics', 'https://trachuan.vn/connect', 'https://trachuan.vn/sources', 'https://trachuan.vn/privacy', 'https://trachuan.vn/terms', 'https://trachuan.vn/vn/topics/1'])
      } else {
        expect(sitemap.headers()['cache-control']).toBe('no-cache')
        expect(sitemap.headers().etag).toBeUndefined()
      }
      if (!available) expect((await context.request.get(`${origin}/vn/topics/1`)).status()).toBe(503)
      await context.close()
    })
  })
}
