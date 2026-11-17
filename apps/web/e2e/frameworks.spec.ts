import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'
import { fulfillFrameworkFixture } from './framework-route'

test.beforeEach(async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
  await page.route('**/api/v1/frameworks**', route => fulfillFrameworkFixture(route, page))
})
test.afterEach(async ({ page }) => { await page.unrouteAll({ behavior: 'wait' }) })
async function fixture(context: import('@playwright/test').BrowserContext, mode: string, signedIn = true) {
  await context.addCookies([
    { name: 'framework-fixture', value: mode, url: 'http://127.0.0.1:3031' },
    ...(signedIn ? [{ name: 'account-fixture', value: '1', url: 'http://127.0.0.1:3031' }] : []),
  ])
}
const control = '/frameworks/fw-alpha/2.1/control?citation=A+%281%29%2F%23%25'

test('redirects a signed-out control visit and sends private headers', async ({ page, context }) => {
  await fixture(context, 'mixed', false)
  const response = await page.request.get(control, { maxRedirects: 0, headers: { 'X-Forwarded-For': clientAddress(test.info()) } })
  expect(response.status()).toBe(302)
  expect(response.headers()['cache-control']).toContain('no-store')
  expect(response.headers()['x-robots-tag']).toBe('noindex')
  const next = new URL(response.headers().location!, 'http://localhost').searchParams.get('next')
  expect(new URL(next!, 'http://localhost').searchParams.get('citation')).toBe('A (1)/#%')
})

test('renders the fixed 404 when the flag is off', async ({ page, context }) => {
  await fixture(context, 'off')
  for (const path of ['/frameworks', '/frameworks/search', '/frameworks/fw-alpha/2.1', control]) {
    const response = await page.goto(path)
    expect(response?.status()).toBe(404)
    await expect(page.getByRole('heading', { name: 'Không có trang này.' })).toBeVisible()
  }
})

for (const mode of ['reduced', 'mixed', 'full']) {
  test(`renders ${mode} records through SSR and navigation`, async ({ page, context }) => {
    await fixture(context, mode)
    const response = await page.goto('/frameworks')
    expect(response?.headers()['cache-control']).toContain('no-store')
    expect(response?.headers()['x-robots-tag']).toBe('noindex')
    await expect(page.getByRole('link', { name: 'fw-alpha 2.1' })).toBeVisible()
    await page.getByRole('link', { name: 'fw-alpha 2.1' }).click()
    await expect(page.getByText('Synthetic fw-alpha')).toBeVisible()
    await page.getByRole('link', { name: /A \(1\)/ }).click()
    await expect(page.getByRole('heading', { name: /A \(1\)/ })).toBeVisible()
    if (mode === 'full') await expect(page.getByText('Synthetic publisher body')).toBeVisible()
    else {
      await expect(page.getByText('Nội dung của nhà xuất bản cần được cấp quyền mới đọc được. Tài khoản này chưa được cấp quyền.')).toBeVisible()
      const html = await (await page.request.get(control, { headers: { 'X-Forwarded-For': clientAddress(test.info()) } })).text()
      for (const field of ['title_original', 'Synthetic publisher title', 'Synthetic publisher body', '"body"', '"note"', '"table"', 'snippet']) expect(html).not.toContain(field)
    }
    await page.goto('/frameworks/search')
    await page.getByLabel('Tra cứu', { exact: true }).fill('Synthetic query')
    await page.getByRole('button', { name: 'Tra cứu', exact: true }).click()
    await expect(page.getByText('Tra theo số hiệu').first()).toBeVisible()
  })
}

test('probes once per tab and hides the corpus switch after failure', async ({ page, context }) => {
  await fixture(context, 'mixed')
  let probes = 0
  await page.route('**/api/v1/frameworks', async (route) => { probes++; await route.fallback() })
  await page.goto('/vn/search')
  await expect(page.locator('select').filter({ hasText: 'Khung kiểm soát' })).toBeVisible()
  await page.reload()
  await expect(page.locator('select').filter({ hasText: 'Khung kiểm soát' })).toBeVisible()
  expect(probes).toBe(1)
  await page.locator('select').filter({ hasText: 'Khung kiểm soát' }).selectOption('frameworks')
  await expect(page).toHaveURL(/\/frameworks\/search$/)
  const other = await context.newPage()
  await other.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(test.info()) })
  await other.route('**/api/v1/frameworks', route => route.fulfill({ status: 503, json: { error: { code: 'unavailable' } } }))
  await other.goto('/vn/search')
  await expect(other.locator('select').filter({ hasText: 'Khung kiểm soát' })).toHaveCount(0)
  await expect(other.getByRole('alert')).toHaveCount(0)
})

for (const width of [320, 390, 1366]) for (const colorScheme of ['light', 'dark'] as const) {
  test(`fits ${width}px in ${colorScheme} theme`, async ({ page, context }) => {
    await fixture(context, 'mixed')
    await page.setViewportSize({ width, height: 900 })
    await page.emulateMedia({ colorScheme })
    for (const path of ['/frameworks', '/frameworks/search', '/frameworks/fw-alpha/2.1', control]) {
      const response = await page.goto(path)
      expect(response?.headers()['cache-control']).toContain('no-store')
      expect(response?.headers()['x-robots-tag']).toBe('noindex')
      if (path.startsWith('/frameworks/fw-alpha/2.1')) {
        const band = page.locator('header.reader-identity.fw-identity')
        await expect(band).toBeVisible()
        const meta = (await band.locator('.fw-meta').first().innerText()).trim()
        expect(meta).not.toMatch(/^[·\s]|[·\s]$/)
        expect(meta).not.toContain('· ·')
      }
      for (const line of await page.locator('.fw-meta').all()) {
        const text = (await line.innerText()).trim()
        if (text) expect(text).not.toMatch(/^[·\s]|[·\s]$/)
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    }
  })
}
