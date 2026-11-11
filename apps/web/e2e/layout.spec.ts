import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'


test('language and device theme control the layout', async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress({ ...testInfo, testIndex: 5 }) })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'English' })).toBeVisible()
  const dark = await page.locator('body').evaluate(el => getComputedStyle(el).backgroundColor)
  await page.getByRole('button', { name: 'English' }).click()
  await expect(page.getByRole('button', { name: 'Tiếng Việt' })).toBeVisible()
  await page.emulateMedia({ colorScheme: 'light' })
  const light = await page.locator('body').evaluate(el => getComputedStyle(el).backgroundColor)
  expect(light).not.toBe(dark)
})
