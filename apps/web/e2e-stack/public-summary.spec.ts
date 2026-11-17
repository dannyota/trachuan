import { expect, test } from '@playwright/test'
import { freshEmail, signIn } from './identity'

const key = 'vn:fixture:public-summary'
const markers = ['E2E-BODY-MARKER-7Q4', 'E2E-REJECTED-HEADING-9K2']

test.skip(process.env.E2E_PUBLIC_SUMMARIES !== '1', 'public summaries are off in this stack')
test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.15' } })

test('a built summary is public, shows only approved fields and is in the sitemap', async ({ page }) => {
  const bodies: Promise<{ url: string, body: string, error?: string }>[] = []
  page.on('response', (received) => {
    bodies.push(received.body().then(body => ({ url: received.url(), body: body.toString('utf8') }), error => ({ url: received.url(), body: '', error: String(error) })))
  })
  const response = await page.goto(`/vn/documents/${key}/summary`)
  expect(response?.status()).toBe(200)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Văn bản tóm tắt E2E')
  await expect(page.getByText('Điều khoản tóm tắt công khai E2E')).toBeVisible()
  await expect(page.getByRole('link', { name: /Chủ đề tóm tắt E2E/ })).toHaveAttribute('href', '/vn/topics/99.1')
  await expect(page.getByRole('banner').getByRole('link', { name: 'Đăng nhập' })).toBeVisible()
  await page.waitForLoadState('networkidle')
  const html = await response!.text()
  const decoded = ['__NUXT_CSP_DATA__', '__NUXT_CSP_CONFIG__'].map((id) => {
    const value = new RegExp(`<meta id="${id}"[^>]*? content="([^"]+)"`).exec(html)?.[1]
    expect(value).toBeTruthy()
    return Buffer.from(value!, 'base64').toString('utf8')
  })
  expect(decoded.join('')).toContain('Văn bản tóm tắt E2E')
  const received = await Promise.all(bodies)
  expect(received.every(item => !item.error)).toBe(true)
  const api = await page.request.get(`/api/v1/public/documents/${encodeURIComponent(key)}`)
  expect(api.status()).toBe(200)
  for (const body of [html, ...decoded, ...received.map(item => item.body), await api.text()]) {
    for (const marker of markers) expect(body).not.toContain(marker)
  }
  const sitemap = await page.request.get('/sitemap-documents.xml')
  expect(sitemap.status()).toBe(200)
  expect(await sitemap.text()).toContain(`/vn/documents/${key}/summary</loc>`)
  await signIn(page, freshEmail())
  await page.goto(`/vn/documents/${key}`)
  for (const marker of markers) await expect(page.locator('.reader-unit').filter({ hasText: marker })).toHaveCount(1)
})
