import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'


test('personal data never enters the public HTML cache', async ({ browser }, testInfo) => {
  const address = clientAddress({ ...testInfo, testIndex: 6 })
  for (let i = 0; i < 2; i++) {
    const context = await browser.newContext({
      extraHTTPHeaders: {
        'X-Forwarded-For': address,
        Cookie: `__Host-session=session${i}`,
      },
    })
    const page = await context.newPage()
    await page.route('**/api/v1/me', route => route.fulfill({ json: { user: { email: `private${i}@example.test` } } }))
    await page.goto('/')
    await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
    await context.close()
  }
  const context = await browser.newContext({ extraHTTPHeaders: { 'X-Forwarded-For': address } })
  const page = await context.newPage()
  await page.route('**/api/v1/me', route => route.fulfill({ status: 401 }))
  const response = await page.goto('/')
  const html = await response!.text()
  expect(html).not.toContain('private0@example.test')
  expect(html).not.toContain('private1@example.test')
  await expect(page.getByRole('link', { name: /sign in|đăng nhập/i })).toBeVisible()
  await context.close()
})
