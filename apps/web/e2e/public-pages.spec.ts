import { expect, test } from '@playwright/test'

for (const language of ['vi', 'en']) {
  test(`public pages are anonymous and cached in ${language}`, async ({ browser }) => {
    const context = await browser.newContext({ extraHTTPHeaders: { Cookie: `lang=${language}; __Host-session=private-account` } })
    const page = await context.newPage()
    const calls: string[] = []
    page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) calls.push(new URL(request.url()).pathname) })
    await page.route('**/api/v1/me', route => route.fulfill({ status: 401 }))
    for (const path of ['/', '/connect', '/sources', '/privacy', '/terms']) {
      const response = await page.goto(`${path}?privatequery=secret-query-text`)
      expect(response?.status()).toBe(200)
      await expect(page.locator('html')).toHaveAttribute('lang', language)
      expect(response?.headers()['cache-control']).toContain('no-cache')
      const html = await response!.text()
      expect(html).not.toContain('secret-query-text')
      expect(html).not.toContain('private-account')
      await expect(page.getByRole('banner').getByRole('link', { name: /sign in|đăng nhập/i })).toBeVisible()
      if (path === '/' || path === '/sources') await expect(page.getByText(/coverage unavailable|chưa có thông tin phạm vi/i)).toBeVisible()
    }
    expect(calls.every(path => path === '/api/v1/me')).toBe(true)
    const text = await context.request.get('/llms.txt?privatequery=secret-query-text')
    expect(text.headers()['content-type']).toContain('text/plain')
    expect(await text.text()).not.toContain('secret-query-text')
    await context.close()
  })
}

test('login errors use fixed messages and never echo the query', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ status: 401 }))
  for (const [code, expected] of [
    ['login_failed', 'Đăng nhập không thành công'],
    ['login_expired', 'Phiên đăng nhập đã hết hạn'],
    ['account_blocked', 'Tài khoản đã bị khóa'],
    ['signup_closed', 'chưa mở đăng ký'],
    ['attacker-input', 'Không thể hoàn tất đăng nhập'],
  ]) {
    const response = await page.goto(`/login?error=${code}`)
    await expect(page.getByText(expected, { exact: false })).toBeVisible()
    if (code === 'attacker-input') expect(await response!.text()).not.toContain(code)
    await expect(page.getByRole('link', { name: /đăng nhập Google/i })).toBeVisible()
  }
})
