import { expect, test, type Page } from '@playwright/test'
import { clientAddress } from './client-address'
import { openAccount } from './account-fixture'

const date = '2026-01-01T00:00:00Z'
const codes = ['ABCDEFGHIJKLMNOPQRSTUVWXYZ']
test.beforeEach(async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
})
async function setup(page: Page, kind: string, failList = false) {
  await openAccount(page)
  let changed = false
  const reads = new Set<string>()
  const enrolled = kind !== 'confirm' && kind !== 'set'
  await page.context().addCookies([{ name: 'account-security', value: `${kind === 'set' ? 0 : 1}${Number(enrolled)}1`, url: 'http://127.0.0.1:3031' }])
  await page.route('**/api/v1/me', route => {
    if (changed) reads.add('me')
    return route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: date, csrf_token: changed ? 'new' : 'csrf', password: kind !== 'set' || changed, google: kind === 'set', totp: enrolled, totp_enrollment: true } })
  })
  for (const list of ['sessions', 'tokens', 'grants']) {
    await page.route(`**/api/v1/me/${list}`, route => {
      if (changed) reads.add(list)
      if (changed && failList && list === 'grants') return route.fulfill({ status: 503, json: { error: { code: 'unavailable', message: 'fixed' } } })
      const sessions = (changed ? [3] : [1, 2]).map((id, index) => ({ id, current: index === 0, created_at: date, last_seen_at: date }))
      const tokens = [{ id: 1, name: 'Old token', created_at: date, expires_at: '2026-04-01T00:00:00Z', last_used_at: null, revoked_at: changed ? date : null }]
      const grants = changed ? [] : [{ id: 1, client_name: 'Old grant', client_id_host: 'example.invalid', created_at: date, last_used_at: null }]
      return route.fulfill({ json: list === 'sessions' ? sessions : list === 'tokens' ? tokens : grants })
    })
  }
  await page.route('**/api/v1/me/totp/setup', route => route.fulfill({ json: { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP' } }))
  const path = kind === 'password' || kind === 'set' ? 'password' : kind === 'confirm' ? 'totp/confirm' : kind === 'remove' ? 'totp' : 'recovery-codes'
  await page.route(`**/api/v1/me/${path}`, async route => {
    changed = true
    if (kind === 'set') await page.context().addCookies([{ name: 'account-security', value: '101', url: 'http://127.0.0.1:3031' }])
    await route.fulfill({ json: kind === 'confirm' || kind === 'recovery' ? { recovery_codes: codes } : {} })
  })
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Old grant', exact: true })).toBeVisible()
  return reads
}
async function mutate(page: Page, kind: string) {
  if (kind === 'password') {
    const password = page.getByRole('region', { name: 'Password: Set', exact: true })
    await password.getByRole('button', { name: 'Change password' }).click()
    await password.getByLabel('Current password', { exact: true }).fill('synthetic current password')
    await password.getByLabel('New password', { exact: true }).fill('synthetic replacement password')
    await password.getByRole('button', { name: 'Save password' }).click()
  } else {
    const authenticator = page.getByRole('region', { name: kind === 'confirm' ? 'Two-step verification: Off' : 'Two-step verification: On', exact: true })
    await authenticator.getByRole('button', { name: kind === 'confirm' ? 'Enable' : kind === 'remove' ? 'Disable' : 'Generate new recovery codes', exact: true }).click()
    await authenticator.getByLabel('Authenticator code', { exact: true }).fill('123456')
    await authenticator.getByRole('button', { name: kind === 'confirm' ? 'Confirm authenticator' : kind === 'remove' ? 'Disable' : 'Generate new recovery codes', exact: true }).click()
  }
}
async function refreshed(page: Page) {
  const sessions = page.locator('section.account-card').filter({ has: page.getByRole('heading', { name: 'Sessions', exact: true }) })
  await expect(sessions.locator('.account-row')).toHaveCount(1)
  await expect(page.getByRole('heading', { name: 'Old grant', exact: true })).toHaveCount(0)
  const token = page.locator('.account-row').filter({ has: page.getByRole('heading', { name: 'Old token', exact: true }) })
  await expect(token.locator('.account-revoked')).toHaveText('Revoked')
}
for (const kind of ['password', 'confirm', 'remove', 'recovery']) {
  test(`${kind} refreshes me and credential lists without replacing the document`, async ({ page }) => {
    const reads = await setup(page, kind)
    await page.evaluate(() => { (window as unknown as { securityDocument: boolean }).securityDocument = true })
    await mutate(page, kind)
    await expect.poll(() => ['me', 'sessions', 'tokens', 'grants'].every(path => reads.has(path))).toBe(true)
    await refreshed(page)
    expect(await page.evaluate(() => (window as unknown as { securityDocument: boolean }).securityDocument)).toBe(true)
    if (kind === 'confirm' || kind === 'recovery') await expect(page.locator('.recovery-codes')).toContainText('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
  })
}
test('a mailed password set opens the refreshed account lists', async ({ page }) => {
  const reads = await setup(page, 'set')
  await page.route('**/api/v1/auth/link', route => route.fulfill({ json: { purpose: 'add_password', email: 'test@example.invalid' } }))
  await page.goto('/link#synthetic-link-token')
  await page.getByLabel('New password', { exact: true }).fill('synthetic replacement password')
  await page.getByRole('button', { name: 'Save password' }).click()
  await expect(page).toHaveURL(/\/account$/)
  await expect(page.getByRole('region', { name: 'Password: Set', exact: true })).toBeVisible()
  await expect.poll(() => ['sessions', 'tokens', 'grants'].every(path => reads.has(path))).toBe(true)
  await refreshed(page)
})
test('a failed list refresh preserves newly issued recovery codes', async ({ page }) => {
  const reads = await setup(page, 'recovery', true)
  await mutate(page, 'recovery')
  await expect.poll(() => ['me', 'sessions', 'tokens', 'grants'].every(path => reads.has(path))).toBe(true)
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.locator('.recovery-codes')).toContainText('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
})
