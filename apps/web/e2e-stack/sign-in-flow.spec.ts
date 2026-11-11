import { expect, test } from '@playwright/test'
import { authorize, freshEmail } from './identity'

test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.13' } })

test('signs in and returns to the requested account page', async ({ page }) => {
  const email = freshEmail()
  await page.goto('/account')
  await authorize(page, email)
  await expect(page).toHaveURL(/\/account$/)
  await expect(page.getByText(email, { exact: true })).toBeVisible()
})

test('preserves the reader release in next', async ({ page }) => {
  await page.goto('/vn/documents/vn:fixture:1?release=20990101&discard=private')
  await authorize(page, freshEmail())
  await expect(page).toHaveURL(/\/vn\/documents\/vn:fixture:1\?release=20990101$/)
})

test('rejects an unverified email', async ({ page }) => {
  await page.goto('/account')
  await authorize(page, freshEmail(), false)
  await expect(page).toHaveURL(/\/login\?error=login_failed$/)
  await expect(page.getByText('Đăng nhập không thành công. Vui lòng thử lại.')).toBeVisible()
})
