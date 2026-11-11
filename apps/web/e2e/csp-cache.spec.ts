import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'


test('cached and fresh pages hydrate and navigate without CSP violations', async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress({ ...testInfo, testIndex: 4 }) })
  const violations: string[] = []
  page.on('console', message => {
    if (message.text().includes('Content Security Policy')) violations.push(message.text())
  })
  for (let i = 0; i < 2; i++) {
    const response = await page.goto('/')
    expect(response?.headers()['content-security-policy']).toContain("script-src 'self'")
    await page.getByRole('button', { name: 'English' }).click()
    await expect(page.getByRole('button', { name: 'Tiếng Việt' })).toBeVisible()
    await page.getByRole('link', { name: 'Connect' }).click()
    await expect(page).toHaveURL(/\/connect$/)
    await page.context().clearCookies()
  }
  expect(violations).toEqual([])
})
