import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { clientAddress } from './client-address'
import { hydrated } from './hydrated'

async function switchToEnglish(page: Page, testInfo: TestInfo) {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
  await page.goto('/')
  await hydrated(page)
  const reload = page.waitForRequest(request => request.isNavigationRequest() && new URL(request.url()).pathname === '/')
  await page.getByRole('button', { name: 'English' }).click()
  expect(await (await reload).headerValue('cookie')).toContain('lang=en')
  await expect(page.getByRole('button', { name: 'Tiếng Việt' })).toBeVisible()
}

test('the language switch waits for a late cookie store write and the reload carries it', async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    const store = (window as unknown as { cookieStore: { set: (options: unknown) => Promise<void> } }).cookieStore
    const original = store.set.bind(store)
    store.set = async (options: unknown) => {
      sessionStorage.setItem('cookieSets', String(Number(sessionStorage.getItem('cookieSets') ?? 0) + 1))
      await new Promise(resolve => setTimeout(resolve, 300))
      await original(options)
    }
  })
  await switchToEnglish(page, testInfo)
  expect(await page.evaluate(() => sessionStorage.getItem('cookieSets'))).toBe('1')
})

test('the language switch falls back to document.cookie without a cookie store', async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'cookieStore', { configurable: true, value: undefined })
  })
  await switchToEnglish(page, testInfo)
})
