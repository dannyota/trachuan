import { expect, test } from '@playwright/test'
import { openAccount, secret } from './account-fixture'

test.use({ channel: 'chromium', launchOptions: { ignoreDefaultArgs: ['--disable-back-forward-cache'] } })

test('a token is absent after document navigation and Back', async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    window.addEventListener('pageshow', event => {
      (window as unknown as { restoredFromCache: boolean }).restoredFromCache = event.persisted
    })
  })
  await openAccount(page)
  await page.getByRole('button', { name: 'Create token', exact: true }).click()
  await page.getByLabel('Name').fill('test')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByRole('dialog').getByText(secret)).toBeVisible()
  const originalType = await page.evaluate(() => (performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming).type)
  await page.evaluate(() => { delete (window as unknown as { restoredFromCache?: boolean }).restoredFromCache })
  await page.goto('/connect')
  await page.goBack({ waitUntil: 'commit' })
  await page.waitForFunction(() => typeof (window as unknown as { restoredFromCache?: boolean }).restoredFromCache === 'boolean')
  await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible()
  const restoration = await page.evaluate(() => ({
    persisted: (window as unknown as { restoredFromCache: boolean }).restoredFromCache,
    type: (performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming).type,
    reasons: (performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming & { notRestoredReasons?: unknown }).notRestoredReasons,
  }))
  testInfo.annotations.push({ type: 'bfcache', description: restoration.persisted ? 'restored' : JSON.stringify(restoration.reasons ?? 'notRestoredReasons unavailable') })
  expect(restoration.persisted, `bfcache was not restored: ${JSON.stringify(restoration.reasons ?? 'notRestoredReasons unavailable')}`).toBe(true)
  expect(restoration.type).toBe(originalType)
  await expect(page.getByText(secret)).toHaveCount(0)
})
