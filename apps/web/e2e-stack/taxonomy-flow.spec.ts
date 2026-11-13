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
  const first = page.locator('.result-card').first()
  await expect(first).toContainText('Chương thử nghiệm 90.1.c1')
  await expect(page.getByText(/Kết quả ngoài bộ lọc: [1-9]/)).toBeVisible()
  await expect(page.locator('.search-results p').filter({ hasText: /Kết quả ngoài bộ lọc: [1-9]/ }).locator('+ .result-card')).toBeVisible()
  const cards = page.locator('.result-card')
  const readerLinks = cards.getByRole('link', { name: 'Đọc trong văn bản' })
  const before = await readerLinks.evaluateAll(links => links.map(link => link.getAttribute('href')))
  expect(before.every(Boolean)).toBe(true)
  await expect(page.getByRole('button', { name: 'Xem thêm kết quả' })).toBeVisible()
  await page.getByRole('button', { name: 'Xem thêm kết quả' }).click()
  await expect.poll(() => readerLinks.count()).toBeGreaterThan(before.length)
  const after = await readerLinks.evaluateAll(links => links.map(link => link.getAttribute('href')))
  const appended = after.slice(before.length)
  expect(after.slice(0, before.length)).toEqual(before)
  expect(appended.every(target => target && !before.includes(target))).toBe(true)
  expect(new Set(appended).size).toBe(appended.length)
  const state = await page.evaluate(() => JSON.stringify({ location: location.href, history: history.state, local: { ...localStorage }, session: { ...sessionStorage }, title: document.title }))
  expect(state).not.toContain(privateText)
  await first.getByRole('link', { name: 'Đọc trong văn bản' }).click()
  await expect(page.locator('.reader-rail')).toContainText('Chương thử nghiệm 90.1.c1')
  expect(leaked).toEqual([])
})
