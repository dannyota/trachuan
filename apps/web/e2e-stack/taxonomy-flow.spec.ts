import { expect, test } from '@playwright/test'
import { authorize, freshEmail } from './identity'

test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.14' } })

test('topic filter survives login, pages results and opens the reader', async ({ page }) => {
  await page.goto('/vn/topics')
  await page.getByRole('link', { name: 'Chủ đề thử nghiệm 90', exact: true }).click()
  await page.getByRole('link', { name: 'Đề mục thử nghiệm 90.1', exact: true }).click()
  await page.getByRole('link', { name: 'Tra cứu trong chủ đề này' }).click()
  await authorize(page, freshEmail())
  await expect(page).toHaveURL(/\/vn\/search\?node=90\.1$/)
  await expect(page.getByRole('button', { name: 'Đề mục thử nghiệm 90.1 ×' })).toBeVisible()

  const privateText = 'picker-private-needle-38927'
  const leaked: string[] = []
  page.on('request', request => {
    if (decodeURIComponent(request.url()).includes(privateText) || (request.postData() || '').includes(privateText)) leaked.push(request.url())
  })
  await page.getByLabel('Tìm chủ đề').fill(privateText)
  await page.getByLabel('Câu hỏi tiếng Việt').fill('Văn bản thử nghiệm\nMẫu phân loại giả lập số 1.')
  await page.getByRole('button', { name: 'Tra cứu', exact: true }).click()
  const first = page.locator('.search-card').first()
  await expect(first).toContainText('Chương thử nghiệm 90.1.c1')
  await expect(page.getByText(/Ngoài bộ lọc: [1-9]/)).toBeVisible()
  await expect(page.locator('.search-results p').filter({ hasText: /Ngoài bộ lọc: [1-9]/ }).locator('+ .search-card')).toBeVisible()
  const cards = page.locator('.search-card')
  const before = await cards.count()
  await expect(page.getByRole('button', { name: 'Xem thêm' })).toBeVisible()
  await page.getByRole('button', { name: 'Xem thêm' }).click()
  await expect.poll(() => cards.count()).toBeGreaterThan(before)
  const state = await page.evaluate(() => JSON.stringify({ location: location.href, history: history.state, local: { ...localStorage }, session: { ...sessionStorage }, title: document.title }))
  expect(state).not.toContain(privateText)
  await first.getByRole('link', { name: 'Đọc văn bản' }).click()
  await expect(page.locator('.reader-rail')).toContainText('Chương thử nghiệm 90.1.c1')
  expect(leaked).toEqual([])
})
