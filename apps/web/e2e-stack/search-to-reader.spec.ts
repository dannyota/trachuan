import { expect, test } from '@playwright/test'
import { freshEmail, signIn } from './identity'

test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.12' } })

test('search opens the anchored fixture and its related document', async ({ page }) => {
  await signIn(page, freshEmail())
  await page.goto('/vn/search')
  await page.getByLabel('Câu hỏi tiếng Việt').fill('Văn bản thử nghiệm\nMẫu giả lập vườn thử nghiệm xanh')
  await page.getByRole('button', { name: 'Tra cứu', exact: true }).click()
  const first = page.locator('.result-card').first()
  await expect(first).toContainText('1/2099/TT-TEST')
  await expect(first).toContainText('Văn bản giả lập thử nghiệm')
  await expect(first).toContainText('28/2099/TT-TEST sửa đổi, bổ sung Điều 1')
  const link = first.getByRole('link', { name: 'Đọc trong văn bản' })
  await expect(link).toHaveAttribute('href', /vn:fixture:1\?release=.*#dieu-1$/)
  await link.click()
  await expect(page.locator('[data-anchor="dieu-1"]')).toContainText('Mẫu giả lập vườn thử nghiệm xanh')
  await expect(page.locator('[data-anchor="dieu-1"]')).toContainText('28/2099/TT-TEST sửa đổi, bổ sung Điều 1')
  await expect(page.locator('.reader-identity')).toContainText('1/2099/TT-TEST')
  await expect(page.locator('.reader-identity')).toContainText('Văn bản giả lập thử nghiệm')
  await expect(page.locator('.reader-identity')).toContainText('Bản phát hành 02/03/2099')
  await expect(page.locator('.reader-rail')).toContainText('Bản phát hành: 02/03/2099')
  await expect(page.locator('.reader-rail')).toContainText(`SHA-256: ${'a'.repeat(64)}`)
  await page.locator('.reader-rail').getByRole('link', { name: '2/2099/TT-TEST', exact: true }).click()
  await expect(page).toHaveURL(/\/vn\/documents\/vn:fixture:2\?release=/)
  await expect(page.locator('.reader-identity')).toContainText('2/2099/TT-TEST')
})
