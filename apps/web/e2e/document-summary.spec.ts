import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'
import { withSummarySite } from './summary-fixture'
import { forbiddenMarkers, publicSummary } from './summary-data'

for (const credential of ['', 'valid', 'expired', 'forged']) {
  test(`summary SSR, hydration and navigation stay public with ${credential || 'no cookie'}`, async ({ browser }, testInfo) => {
    await withSummarySite(async (fixtureOrigin, state) => {
      const origin = credential ? fixtureOrigin.replace('127.0.0.1', 'localhost') : fixtureOrigin
      const context = await browser.newContext({ extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(testInfo) } })
      try {
        const page = await context.newPage()
        const violations: string[] = []
        const browserCalls: string[] = []
        const responses: Promise<{ url: string, body: string, error?: string }>[] = []
        await page.exposeFunction('recordSummaryCsp', (value: string) => violations.push(value))
        await page.addInitScript(() => document.addEventListener('securitypolicyviolation', event => (window as unknown as { recordSummaryCsp: (value: string) => void }).recordSummaryCsp(event.violatedDirective)))
        page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) browserCalls.push(new URL(request.url()).pathname) })
        await context.route('**/*', async (route) => {
          const capture = (async () => {
            const headers = await route.request().allHeaders()
            const response = await route.fetch({ headers: { ...headers, cookie: headers.cookie ?? '' }, maxRedirects: 0 })
            const body = await response.text()
            await route.fulfill({ response })
            return { url: route.request().url(), body }
          })().catch(async (error) => {
            await route.abort()
            return { url: route.request().url(), body: '', error: String(error) }
          })
          responses.push(capture)
          await capture
        })
        if (credential) {
          const primed = await page.goto(`${origin}/__fixture/session?value=${credential}`)
          expect(primed?.status()).toBe(200)
          expect((await context.cookies()).find(cookie => cookie.name === '__Host-session')).toMatchObject({ value: credential, domain: 'localhost', path: '/', secure: true, httpOnly: true, sameSite: 'Lax' })
        }
        const response = await page.goto(`${origin}/vn/documents/vn:vbpl:1/summary`)
        expect(response?.status()).toBe(200)
        await expect(page.getByRole('heading', { level: 1 })).toHaveText(publicSummary.document.title)
        await expect(page.getByText('Chương được duyệt', { exact: false })).toBeVisible()
        await expect(page.getByText('Điều được duyệt', { exact: false })).toBeVisible()
        await expect(page.getByRole('link', { name: '2/2099/QH Văn bản liên quan' })).toBeVisible()
        await expect(page.getByRole('banner').getByRole('link', { name: 'Đăng nhập' })).toBeVisible()
        expect(await page.locator('.reader-unit').count()).toBe(0)
        const html = await response!.text()
        const decoded = ['__NUXT_CSP_DATA__', '__NUXT_CSP_CONFIG__'].map(id => Buffer.from(new RegExp(`<meta id="${id}"[^>]*? content="([^"]+)"`).exec(html)![1]!, 'base64').toString('utf8'))
        expect(decoded.every(value => value.length > 0)).toBe(true)
        const blocks = await page.locator('script[type="application/ld+json"]').allTextContents()
        expect(blocks).toHaveLength(1)
        expect(JSON.parse(blocks[0]!)['@graph'][0].name).toBe(publicSummary.document.title)
        await page.getByRole('link', { name: /2\/2099\/QH/ }).click()
        await expect(page).toHaveURL(`${origin}/vn/documents/vn:vbpl:2/summary`)
        await expect(page.getByRole('heading', { level: 1 })).toHaveText(publicSummary.document.title)
        await page.getByRole('link', { name: 'UNAVAILABLE' }).click()
        await expect(page.getByRole('heading', { level: 1 })).toHaveText('Tra Chuẩn đang gián đoạn.')
        await page.goBack()
        await expect(page.getByRole('link', { name: 'MISSING' })).toBeVisible()
        await page.getByRole('link', { name: 'MISSING' }).click()
        await expect(page.getByRole('heading', { level: 1 })).toHaveText('Không có trang này.')
        await page.waitForLoadState('networkidle')
        const payloads = await Promise.all(responses)
        for (const payload of payloads) expect(payload.error, payload.url).toBeUndefined()
        const bodies = [html, ...decoded, ...payloads.map(payload => payload.body)]
        for (const body of bodies) for (const marker of forbiddenMarkers) expect(body).not.toContain(marker)
        const forbidden = /^\/api\/v1\/(?:me(?:\/|$)|documents\/|read(?:$|\/)|search(?:$|\/)|auth\/)/
        expect([...browserCalls, ...state.calls.map(call => call.path)].filter(path => forbidden.test(path))).toEqual([])
        expect(state.calls.every(call => call.cookie === '')).toBe(true)
        if (credential) expect(state.pageCookies.some(cookie => cookie.includes(`__Host-session=${credential}`))).toBe(true)
        expect(violations).toEqual([])
      } finally { await context.close() }
    })
  })
}

for (const stateName of ['available', 'flag off', 'rollback', 'no active release']) {
  test(`summary, topic links and sitemap reflect ${stateName}`, async ({ browser, request }, testInfo) => {
    await withSummarySite(async (origin, state) => {
      state.available = stateName === 'available'
      state.release = stateName === 'no active release' ? null : stateName === 'rollback' ? '20981201_1' : publicSummary.release
      const context = await browser.newContext({ extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(testInfo) } })
      try {
        const page = await context.newPage()
        const summary = await page.goto(`${origin}/vn/documents/vn:vbpl:1/summary`)
        expect(summary?.status()).toBe(stateName === 'available' ? 200 : stateName === 'no active release' ? 503 : 404)
        await page.goto(`${origin}/vn/topics/1`)
        if (stateName !== 'no active release') await expect(page.getByRole('link', { name: 'Luật mẫu', exact: true })).toHaveAttribute('href', `/vn/documents/vn:vbpl:1${state.available ? '/summary' : ''}`)
        const sitemap = await request.get(`${origin}/sitemap.xml`, { headers: { 'X-Forwarded-For': clientAddress(testInfo) } })
        expect(sitemap.status()).toBe(stateName === 'no active release' ? 503 : 200)
        expect((await sitemap.text()).includes('<sitemapindex')).toBe(state.available)
      } finally { await context.close() }
    })
  })
}
