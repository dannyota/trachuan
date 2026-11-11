import { expect, test } from '@playwright/test'
import { openAccount, secret } from './account-fixture'

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
  await page.goto('/connect')
  await page.goBack()
  await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible()
  const restoration = await page.evaluate(() => ({
    persisted: (window as unknown as { restoredFromCache: boolean }).restoredFromCache,
    type: (performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming).type,
  }))
  expect(typeof restoration.persisted).toBe('boolean')
  testInfo.annotations.push({ type: 'bfcache', description: restoration.persisted ? 'restored' : 'not restored' })
  expect(restoration.type).toBe(restoration.persisted ? originalType : 'back_forward')
  await expect(page.getByText(secret)).toHaveCount(0)
})
