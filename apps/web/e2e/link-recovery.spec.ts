import { expect, test, type Page } from '@playwright/test'
import { clientAddress } from './client-address'

const token = 'synthetic-link-token'
const message = 'Sign in, then open the link from your mail again.'
const account = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf', password: false, google: true, totp: false, totp_enrollment: true }
test.beforeEach(async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
  await page.context().addCookies([{ name: 'lang', value: 'en', url: 'http://127.0.0.1:3031' }])
})
async function lookup(page: Page, purpose: string, signedIn = true) {
  await page.route('**/api/v1/me', route => route.fulfill(signedIn ? { json: account } : { status: 204 }))
  await page.route('**/api/v1/auth/link', route => route.fulfill({ json: { purpose, email: account.email } }))
}
async function recovery(page: Page) {
  await expect(page.getByRole('alert')).toHaveText(message)
  await expect(page.locator('form')).toHaveCount(0)
  const href = await page.getByRole('main').getByRole('link', { name: 'Sign in', exact: true }).getAttribute('href')
  const destination = new URL(href!, page.url())
  expect(destination.pathname).toBe('/login')
  expect(destination.searchParams.get('next')).toBe('/account')
  expect(destination.searchParams.size).toBe(1)
  await expect(page).toHaveURL(/\/link$/)
  expect(await page.evaluate(() => JSON.stringify(history.state))).not.toContain(token)
  expect(await page.evaluate(() => JSON.stringify([localStorage, sessionStorage]))).not.toContain(token)
  expect(JSON.stringify(await page.context().cookies())).not.toContain(token)
}
test('an add-password link without a session asks for sign-in and reopening the mail', async ({ page }) => {
  await lookup(page, 'add_password', false)
  let completions = 0
  await page.route('**/api/v1/me/password', route => { completions++; return route.fulfill({ json: {} }) })
  await page.goto(`/link#${token}`)
  await recovery(page)
  expect(completions).toBe(0)
})
for (const [code, status, factor] of [['account_changed', 409, false], ['reauth_required', 403, false], ['reauth_required', 403, true]] as const) {
  test(`add-password ${code} with factor=${factor} drops the token and keeps the recovery message`, async ({ page }) => {
    await lookup(page, 'add_password')
    await page.route('**/api/v1/me/password', route => route.fulfill({ status, json: { error: { code, message: 'fixed' }, ...(factor ? { factor: true } : {}) } }))
    await page.goto(`/link#${token}`)
    await page.getByLabel('New password', { exact: true }).fill('synthetic replacement password')
    await page.getByRole('button', { name: 'Save password' }).click()
    await recovery(page)
    await expect(page.getByText('This link has expired or has already been used. Request a new link.', { exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: 'Tiếng Việt', exact: true }).click()
    await expect(page.getByRole('alert')).toHaveText('Đăng nhập, rồi mở lại liên kết trong email.')
    await expect(page.locator('form')).toHaveCount(0)
  })
}
for (const purpose of ['registration', 'reset']) {
  test(`${purpose} completes after switching language without reloading or retaining the token`, async ({ page }) => {
    await lookup(page, purpose)
    await page.context().addCookies([{ name: 'account-fixture', value: '1', url: 'http://127.0.0.1:3031' }])
    await page.route('**/api/v1/me/**', route => route.fulfill({ json: route.request().url().endsWith('/reports') ? { reports: [] } : [] }))
    let body: unknown
    await page.route(`**/api/v1/auth/${purpose === 'registration' ? 'signup' : 'reset'}/complete`, async route => {
      body = route.request().postDataJSON()
      await route.fulfill(purpose === 'reset' ? { status: 204 } : { json: {} })
    })
    await page.goto(`/link#${token}`)
    await page.getByLabel('New password', { exact: true }).fill('synthetic replacement password')
    await page.evaluate(() => { (window as unknown as { linkDocument: boolean }).linkDocument = true })
    await page.getByRole('button', { name: 'Tiếng Việt', exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('lang', 'vi')
    expect(await page.evaluate(() => (window as unknown as { linkDocument: boolean }).linkDocument)).toBe(true)
    await expect(page.getByLabel('Mật khẩu mới', { exact: true })).toHaveValue('synthetic replacement password')
    await expect(page).toHaveURL(/\/link$/)
    expect(await page.evaluate(() => JSON.stringify(history.state))).not.toContain(token)
    await page.getByRole('button', { name: 'Lưu mật khẩu' }).click()
    if (purpose === 'reset') {
      await expect(page.getByRole('status').filter({ hasText: 'Đã lưu mật khẩu.' })).toBeVisible()
      await expect(page.getByRole('heading', { level: 1 })).toHaveText('Đặt lại mật khẩu')
      await expect(page).toHaveTitle('Đặt lại mật khẩu · Tra Chuẩn')
      await page.getByRole('button', { name: 'English', exact: true }).click()
      await expect(page.getByRole('heading', { level: 1 })).toHaveText('Reset password')
      await expect(page).toHaveTitle('Reset password · Tra Chuẩn')
    }
    else await expect(page).toHaveURL(/\/account$/)
    expect(body).toEqual({ token, password: 'synthetic replacement password' })
  })
}

for (const purpose of ['registration', 'add_password', 'reset']) {
  test(`${purpose} has a localized title and a visually hidden account username`, async ({ page }) => {
    await lookup(page, purpose)
    await page.goto(`/link#${token}`)
    await expect(page).toHaveTitle(`${purpose === 'reset' ? 'Reset password' : 'Set password'} · Tra Chuẩn`)
    const username = page.locator('form input[name=username]')
    await expect(username).toHaveAttribute('type', 'text')
    await expect(username).toHaveAttribute('autocomplete', 'username')
    await expect(username).toHaveValue(account.email)
    const bounds = await username.boundingBox()
    expect(bounds?.width).toBeLessThanOrEqual(1)
    expect(bounds?.height).toBeLessThanOrEqual(1)
    await page.getByRole('button', { name: 'Tiếng Việt', exact: true }).click()
    await expect(page).toHaveTitle(`${purpose === 'reset' ? 'Đặt lại mật khẩu' : 'Đặt mật khẩu'} · Tra Chuẩn`)
    await expect(username).toHaveValue(account.email)
  })
}
