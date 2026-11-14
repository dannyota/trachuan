import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'

test.beforeEach(async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
})

for (const language of ['vi', 'en']) {
  test(`legal pages and support contact in ${language}`, async ({ page, context }) => {
    await context.addCookies([{ name: 'lang', value: language, url: 'http://127.0.0.1:3031' }])
    for (const path of ['/privacy', '/terms']) {
      await page.goto(path)
      await expect(page.locator('main h1')).toBeVisible()
      await expect(page.locator('main section')).not.toHaveCount(0)
      await expect(page.locator('footer a[href="mailto:support@trachuan.vn"]')).toBeVisible()
    }
  })
  for (const colorScheme of ['light', 'dark'] as const) {
    test(`Google sign-in in ${language} and ${colorScheme}`, async ({ page, context }) => {
      await context.addCookies([{ name: 'lang', value: language, url: 'http://127.0.0.1:3031' }])
      await page.emulateMedia({ colorScheme })
      await page.goto('/login?next=%2Fconnect')
      const link = page.getByRole('link', { name: language === 'vi' ? 'Tiếp tục với Google' : 'Continue with Google' })
      await expect(link).toHaveAttribute('href', '/api/v1/auth/google/start?next=%2Fconnect')
      const image = link.locator('img')
      await expect(image).toHaveAttribute('src', '/google-g.png')
      await expect(image).toHaveAttribute('alt', '')
      expect(await image.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth > 0)).toBe(true)
      await expect(link).toHaveCSS('background-color', 'rgb(255, 255, 255)')
      await expect(link).toHaveCSS('color', 'rgb(31, 31, 31)')
      await expect(link).toHaveCSS('height', '40px')
      await expect(link).toHaveCSS('border-color', 'rgb(116, 119, 117)')
      await expect(image).toHaveCSS('width', '20px')
      await expect(image).toHaveCSS('height', '20px')
    })
  }
}
