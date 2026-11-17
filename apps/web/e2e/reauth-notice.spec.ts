import { expect, test, type Page } from '@playwright/test'
import { openAccount } from './account-fixture'
import { clientAddress } from './client-address'

test.beforeEach(async ({ page }, testInfo) => { await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) }) })

const googleAccount = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf', password: false, google: true, totp: false, totp_enrollment: true }
const refusal = { error: { code: 'reauth_required', message: 'fixed' } }
const reason = 'Sign in again to continue on your account page.'

async function accountWith(page: Page, password = false) {
  await openAccount(page)
  await page.route('**/api/v1/me', route => route.fulfill({ json: { ...googleAccount, password, google: !password, totp: password } }))
  await page.context().addCookies([{ name: 'account-security', value: password ? '111' : '001', url: 'http://127.0.0.1:3031' }])
  await page.reload()
}

async function createToken(page: Page) {
  await page.getByRole('button', { name: 'Create token' }).click()
  const dialog = page.locator('dialog[open]')
  await dialog.locator('input').first().fill('fixture')
  await dialog.getByRole('button', { name: 'Create', exact: true }).click()
  return dialog
}

const actions = [
  { action: 'token', url: '**/api/v1/me/tokens', message: "To create a token, sign in again. You'll come back here." },
  { action: 'password-link', url: '**/api/v1/me/password/link', message: "To add a password, sign in again. You'll come back here." },
  { action: 'authenticator', url: '**/api/v1/me/totp/setup', message: "To change two-step verification, sign in again. You'll come back here." },
] as const

for (const { action, url, message } of actions) {
  test(`Google-only ${action} refuses in place and resumes after sign-in`, async ({ page }) => {
    await accountWith(page)
    let requests = 0
    await page.route(url, route => {
      if (route.request().method() !== 'POST') return route.fallback()
      requests++
      return route.fulfill({ status: 403, json: refusal })
    })
    const before = page.url()
    const started = Date.now()
    const region = action === 'token' ? await createToken(page) : page.locator(`#reauth-${action}`)
    if (action === 'password-link') await region.getByRole('button', { name: 'Add a password by email' }).click()
    if (action === 'authenticator') await region.getByRole('button', { name: 'Enable', exact: true }).click()
    await expect(region.getByText(message, { exact: true })).toBeVisible()
    await expect(page).toHaveURL(before)
    expect(requests).toBe(1)
    await region.getByRole('button', { name: 'Sign in again', exact: true }).click()
    await expect(page).toHaveURL(url => url.pathname === '/login' && url.searchParams.get('next') === '/account' && url.searchParams.get('reason') === 'reauth')
    await expect(page.getByText(reason, { exact: true })).toBeVisible()
    const pending = await page.evaluate(() => JSON.parse(sessionStorage.getItem('reauth-action') || 'null'))
    expect(pending).toEqual({ action, at: expect.any(Number) })
    expect(pending.at).toBeGreaterThanOrEqual(started)
    expect(pending.at).toBeLessThanOrEqual(Date.now())
    await page.goto('/account')
    if (action === 'token') await expect(page.locator('dialog[open]')).toBeVisible()
    else await expect(page.locator(`#reauth-${action} button`).first()).toBeFocused()
    expect(requests).toBe(1)
    expect(await page.evaluate(() => sessionStorage.getItem('reauth-action'))).toBeNull()
  })
}

test('token creation requiring a factor asks a password account to sign in again', async ({ page }) => {
  await accountWith(page, true)
  await page.route('**/api/v1/me/tokens', route => route.request().method() === 'POST'
    ? route.fulfill({ status: 403, json: { ...refusal, factor: true } })
    : route.fallback())
  const before = page.url()
  const dialog = await createToken(page)
  await expect(dialog.getByText(actions[0].message, { exact: true })).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Sign in again' })).toBeVisible()
  await expect(page).toHaveURL(before)
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.locator('dialog[open]')).toHaveCount(0)
})

test('ignores an unknown login reason', async ({ page }) => {
  await page.context().addCookies([{ name: 'lang', value: 'en', url: 'http://127.0.0.1:3031' }])
  await page.goto('/login?reason=unknown')
  await expect(page.getByText(reason, { exact: true })).toHaveCount(0)
})

for (const [name, value] of [
  ['expired', JSON.stringify({ action: 'token', at: Date.now() - 600001 })],
  ['future', JSON.stringify({ action: 'token', at: Date.now() + 3600000 })],
  ['malformed', '{invalid'],
  ['unknown action', JSON.stringify({ action: 'other', at: Date.now() })],
]) {
  test(`drops ${name} pending actions`, async ({ page }) => {
    await accountWith(page)
    await page.evaluate(value => sessionStorage.setItem('reauth-action', value), value)
    await page.reload()
    await expect(page.locator('dialog[open]')).toHaveCount(0)
    expect(await page.evaluate(() => sessionStorage.getItem('reauth-action'))).toBeNull()
  })
}
