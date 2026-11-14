import { expect, test } from '@playwright/test'
import { freshEmail, signIn } from './identity'

test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.11' } })

test('creates and revokes a token, revokes another session and signs out', async ({ page, browser, baseURL }) => {
  const email = freshEmail()
  await signIn(page, email)
  const otherContext = await browser.newContext({ baseURL, extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.11' } })
  try {
    const other = await otherContext.newPage()
    await signIn(other, email)
    await page.reload()
    const sessions = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Phiên đăng nhập' }) })
    await expect(sessions.locator('.account-row')).toHaveCount(2)
    await sessions.getByRole('button', { name: 'Thu hồi' }).click()
    await expect(sessions.locator('.account-row')).toHaveCount(1)
    await other.reload()
    await expect(other.getByLabel('Email', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Tạo mã mới', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel('Tên').fill('fixture token')
    await dialog.getByRole('button', { name: 'Tạo', exact: true }).click()
    const token = await dialog.locator('.token-value').textContent()
    expect(token?.trim().length).toBeGreaterThan(20)
    const response = await page.request.post('/mcp', {
      headers: { Authorization: `Bearer ${token!.trim()}`, Accept: 'application/json, text/event-stream' },
      data: { jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'report_issue', arguments: { country: 'vn', kind: 'wrong_text', handle: 'vn:fixture:1#dieu-1' } } },
    })
    expect(response.ok()).toBe(true)
    const result = await response.json()
    expect(result.result.structuredContent.state).toBe('open')
    await dialog.getByRole('button', { name: 'Đóng' }).click()
    await page.reload()
    const reports = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Báo lỗi của bạn' }) })
    await expect(reports).toContainText('1/2099/TT-TEST')
    await reports.getByRole('button', { name: 'Rút báo cáo' }).click()
    await expect(reports).toContainText('Đã rút')
    await expect(page.getByText(token!, { exact: true })).toHaveCount(0)
    const row = page.locator('.account-row').filter({ has: page.getByRole('heading', { name: 'fixture token' }) })
    await row.getByRole('button', { name: 'Thu hồi' }).click()
    await expect(row.getByRole('button', { name: 'Thu hồi' })).toHaveCount(0)
    await expect(sessions.getByText('Phiên này')).toBeVisible()
    await page.getByRole('main').getByRole('button', { name: 'Đăng xuất' }).click()
    await expect(page).toHaveURL(/\/$/)
    await page.goto('/account')
    await expect(page.getByLabel('Email', { exact: true })).toBeVisible()
  } finally { await otherContext.close() }
})
