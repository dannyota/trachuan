import { expect, test, type Page } from '@playwright/test'
import { openAccount } from './account-fixture'
import { clientAddress } from './client-address'

test.beforeEach(async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
})

const codes = ['ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'BCDEFGHIJKLMNOPQRSTUVWXYZA']
async function security(page: Page, password = true, totp = false, enrollment = true) {
  await openAccount(page)
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf', password, google: !password, totp, totp_enrollment: enrollment } }))
  await page.context().addCookies([{ name: 'account-security', value: [Number(password), Number(totp), Number(enrollment)].join(''), url: 'http://127.0.0.1:3031' }])
  await page.reload()
}
test('enrolls with an inline QR and shows recovery codes once', async ({ page }) => {
  await security(page)
  await expect(page.getByRole('region', { name: 'Password: Set' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Two-step verification: Off' })).toBeVisible()
  await page.route('**/api/v1/me/totp/setup', route => route.fulfill({ json: { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP' } }))
  await page.route('**/api/v1/me/totp/confirm', route => route.fulfill({ json: { recovery_codes: codes } }))
  await page.getByRole('button', { name: 'Enable', exact: true }).click()
  await expect(page.getByRole('img', { name: 'Scan this QR code with your authenticator app' })).toBeVisible()
  await expect(page.locator('.auth-secret')).toContainText('JBSW Y3DP EHPK 3PXP')
  await page.getByLabel('Authenticator code', { exact: true }).fill('123456')
  await page.getByRole('button', { name: 'Confirm authenticator', exact: true }).click()
  await expect(page.locator('.recovery-codes')).toContainText('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download recovery codes' }).click()
  expect((await download).suggestedFilename()).toBe('trachuan-recovery-codes.txt')
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(page.locator('.recovery-codes')).toHaveCount(0)
  await page.reload()
  await expect(page.locator('.auth-secret')).toHaveCount(0)
  await expect(page.locator('.recovery-codes')).toHaveCount(0)
})
test('enrollment off hides setup but permits removal and code replacement', async ({ page }) => {
  await security(page, true, true, false)
  await expect(page.getByRole('button', { name: 'Enable', exact: true })).toHaveCount(0)
  await page.route('**/api/v1/me/recovery-codes', route => route.fulfill({ json: { recovery_codes: codes } }))
  await expect(page.getByRole('region', { name: 'Password: Set' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Two-step verification: On' })).toBeVisible()
  await page.getByRole('button', { name: 'Generate new recovery codes', exact: true }).click()
  await page.getByRole('region', { name: 'Two-step verification: On' }).getByLabel('Authenticator code', { exact: true }).fill('123456')
  await page.getByRole('button', { name: 'Generate new recovery codes' }).click()
  await expect(page.locator('.recovery-codes')).toBeVisible()
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await page.route('**/api/v1/me/totp', async route => {
    expect(route.request().postDataJSON()).toEqual({ recovery_code: 'ABCDE FGHIJ KLMNO PQRST UVWXY Z' })
    await route.fulfill({ json: {} })
  })
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await page.getByRole('button', { name: 'Disable', exact: true }).click()
  await page.getByRole('button', { name: 'Use a recovery code' }).click()
  await page.getByLabel('Recovery code', { exact: true }).fill('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
  await page.getByRole('button', { name: 'Disable', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Security settings updated.' })).toBeVisible()
})
for (const status of [401, 404, 429]) {
  test(`setup handles ${status}`, async ({ page }) => {
    await security(page)
    await page.route('**/api/v1/me/totp/setup', route => route.fulfill({ status, json: { error: { code: status === 401 ? 'unauthenticated' : status === 404 ? 'not_found' : 'rate_limited', message: 'fixed' } } }))
    await page.getByRole('button', { name: 'Enable', exact: true }).click()
    if (status === 401) await expect(page).toHaveURL(/\/login\?next=/)
    else await expect(page.getByRole('alert')).toContainText(status === 404 ? 'unavailable' : 'Too many')
  })
}
for (const factor of [false, true]) {
  test(`password change reauth handles factor=${factor}`, async ({ page }) => {
    await security(page)
    let attempts = 0
    await page.route('**/api/v1/me/password', async route => {
      attempts++
      if (attempts === 1) return route.fulfill({ status: 403, json: { error: { code: 'reauth_required', message: 'fixed' }, factor } })
      expect(route.request().postDataJSON()).toEqual({ current: 'synthetic current password', password: 'synthetic new password', ...(factor ? { code: '123456' } : {}) })
      await route.fulfill({ json: {} })
    })
    await page.route('**/api/v1/me/reauth', route => route.fulfill({ status: 204 }))
    await page.getByRole('button', { name: 'Change password', exact: true }).click()
    await page.getByLabel('Current password', { exact: true }).fill('synthetic current password')
    await page.getByLabel('New password', { exact: true }).fill('synthetic new password')
    await page.getByRole('button', { name: 'Save password' }).click()
    await expect(page.getByText('Confirm your password to continue')).toBeVisible()
    await page.getByLabel('Password', { exact: true }).fill('synthetic current password')
    if (factor) await page.getByLabel('Authenticator code', { exact: true }).fill('123456')
    await page.getByRole('button', { name: 'Verify', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Security settings updated.' })).toBeVisible()
  })
}
test('Google-only reauth returns through login', async ({ page }) => {
  await security(page, false)
  await page.route('**/api/v1/me/password/link', route => route.fulfill({ status: 403, json: { error: { code: 'reauth_required', message: 'fixed' } } }))
  await page.getByRole('button', { name: 'Add a password by email' }).click()
  await expect(page).toHaveURL(url => url.pathname === '/login' && url.searchParams.get('next') === '/account' && url.search.split('&').length === 1)
})

for (const operation of ['recovery', 'remove'] as const) {
  test(`Enter submits ${operation} without calling the other route`, async ({ page }) => {
    await security(page, true, true)
    const calls: string[] = []
    await page.route('**/api/v1/me/recovery-codes', route => { calls.push(`${route.request().method()} recovery`); return route.fulfill({ json: { recovery_codes: codes } }) })
    await page.route('**/api/v1/me/totp', route => { calls.push(`${route.request().method()} remove`); return route.fulfill({ json: {} }) })
    const name = operation === 'recovery' ? 'Generate new recovery codes' : 'Disable'
    await page.getByRole('button', { name, exact: true }).click()
    const input = page.getByLabel('Authenticator code', { exact: true })
    await input.fill('123456')
    await input.press('Enter')
    await expect.poll(() => calls.length).toBe(1)
    expect(calls).toEqual([operation === 'recovery' ? 'POST recovery' : 'DELETE remove'])
  })
}
