import { expect, test, type Page } from '@playwright/test'
import { clientAddress } from './client-address'

test.beforeEach(async ({ page }, testInfo) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
})

const token = 'synthetic-link-token'
const failure = (code: string) => ({ error: { code, message: code } })
async function english(page: Page) {
  await page.context().addCookies([{ name: 'lang', value: 'en', url: 'http://127.0.0.1:3031' }])
  await page.route('**/api/v1/me', route => route.fulfill({ status: 204 }))
}
test('account pages answer no-store and noindex', async ({ request }) => {
  for (const path of ['/login', '/login/factor', '/signup', '/reset', '/link']) {
    const response = await request.get(path)
    expect(response.status(), path).toBe(200)
    expect(response.headers()['cache-control'], path).toBe('no-store')
    expect(response.headers()['x-robots-tag'], path).toBe('noindex')
  }
})
test('password and Google preserve a consent destination', async ({ page }) => {
    await english(page)
  let body: unknown
  await page.route('**/api/v1/auth/password', async route => { body = route.request().postDataJSON(); await route.fulfill({ json: { next: '/connect' } }) })
  await page.goto('/login?next=%2Foauth%2Fconsent%2Fsynthetic')
  await expect(page.getByRole('link', { name: 'Continue with Google' })).toHaveAttribute('href', '/api/v1/auth/google/start?next=%2Foauth%2Fconsent%2Fsynthetic')
  await page.getByLabel('Email', { exact: true }).fill('person@example.invalid')
  await page.getByLabel('Password', { exact: true }).fill('synthetic long password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page).toHaveURL(/\/connect$/)
  expect(body).toEqual({ email: 'person@example.invalid', password: 'synthetic long password', next: '/oauth/consent/synthetic' })
})
for (const status of [401, 403, 429]) {
  test(`password login shows a fixed error for ${status}`, async ({ page }) => {
    await english(page)
    await page.route('**/api/v1/auth/password', route => route.fulfill({ status, json: failure(status === 429 ? 'rate_limited' : 'login_failed') }))
    await page.goto('/login')
    await page.getByLabel('Email', { exact: true }).fill('person@example.invalid')
    await page.getByLabel('Password', { exact: true }).fill('synthetic long password')
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(page).toHaveURL(/\/login$/)
  })
}
for (const path of ['signup', 'reset']) {
  for (const status of [202, 403, 429]) {
    test(`${path} has uniform mail answers for ${status}`, async ({ page }) => {
    await english(page)
      await page.route(`**/api/v1/auth/${path}`, route => route.fulfill({ status, json: status === 202 ? {} : failure(status === 429 ? 'rate_limited' : 'signup_closed') }))
      await page.goto(`/${path}`)
      await page.getByLabel('Email', { exact: true }).fill('person@example.invalid')
      await page.getByRole('button', { name: 'Send link' }).click()
      if (status === 429) await expect(page.getByRole('alert')).toContainText('Too many')
      else await expect(page.getByRole('status').filter({ hasText: 'Check your mail' })).toBeVisible()
    })
  }
}
test('password factor_required opens the pending factor page', async ({ page }) => {
    await english(page)
  await page.route('**/api/v1/auth/password', route => route.fulfill({ json: { factor_required: true, csrf: 'pending-csrf' } }))
  await page.route('**/api/v1/auth/factor', route => route.fulfill({ json: { csrf: 'pending-csrf', next: '/account' } }))
  await page.goto('/login')
  await page.getByLabel('Email', { exact: true }).fill('person@example.invalid')
  await page.getByLabel('Password', { exact: true }).fill('synthetic long password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page).toHaveURL(/\/login\/factor$/)
})
for (const recovery of [false, true]) {
  test(`pending factor completes with ${recovery ? 'recovery' : 'TOTP'} code`, async ({ page }) => {
    await english(page)
    await page.route('**/api/v1/auth/factor', async route => {
      if (route.request().method() === 'GET') return route.fulfill({ json: { csrf: 'pending-csrf', next: '/connect' } })
      expect(route.request().headers()['x-csrf-token']).toBe('pending-csrf')
      expect(route.request().postDataJSON()).toEqual(recovery ? { recovery_code: 'ABCDE FGHIJ KLMNO PQRST UVWXY Z' } : { code: '123456' })
      await route.fulfill({ json: { next: '/connect' } })
    })
    await page.goto('/login/factor')
    if (recovery) await page.getByRole('button', { name: 'Use a recovery code' }).click()
    await page.getByLabel(recovery ? 'Recovery code' : 'Authenticator code', { exact: true }).fill(recovery ? 'ABCDE FGHIJ KLMNO PQRST UVWXY Z' : '123456')
    await page.getByRole('button', { name: 'Verify', exact: true }).click()
    await expect(page).toHaveURL(/\/connect$/)
  })
}
test('expired pending authentication returns to login', async ({ page }) => {
    await english(page)
  await page.route('**/api/v1/auth/factor', route => route.fulfill({ status: 401, json: failure('factor_failed') }))
  await page.goto('/login/factor')
  await expect(page).toHaveURL(/\/login$/)
})
for (const status of [401, 403, 429]) {
  test(`factor failure ${status} keeps the code form when pending cookie is live`, async ({ page }) => {
    await english(page)
    await page.route('**/api/v1/auth/factor', route => route.fulfill(route.request().method() === 'GET' ? { json: { csrf: 'pending-csrf', next: '/' } } : { status, json: failure(status === 429 ? 'rate_limited' : 'factor_failed') }))
    await page.goto('/login/factor')
    await page.getByLabel('Authenticator code', { exact: true }).fill('000000')
    await page.getByRole('button', { name: 'Verify', exact: true }).click()
    await expect(page.getByRole('alert')).toBeVisible()
  })
}
for (const purpose of ['registration', 'reset', 'add_password']) {
  test(`link fragment is removed before lookup and ${purpose} completion`, async ({ page }) => {
    await english(page)
    let lookupHash = 'not-observed'
    await page.route('**/api/v1/auth/link', async route => {
      lookupHash = await page.evaluate(() => location.hash)
      expect(await page.evaluate(() => JSON.stringify(history.state))).not.toContain(token)
      expect(route.request().postDataJSON()).toEqual({ token })
      await route.fulfill({ json: { purpose, email: 'person@example.invalid' } })
    })
    if (purpose === 'add_password') await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'person@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf', password: false, google: true, totp: false, totp_enrollment: true } }))
    const path = purpose === 'registration' ? 'auth/signup/complete' : purpose === 'reset' ? 'auth/reset/complete' : 'me/password'
    await page.route(`**/api/v1/${path}`, async route => {
      expect(route.request().postDataJSON()).toEqual({ token, password: 'synthetic long password' })
      await route.fulfill(purpose === 'reset' ? { status: 204 } : { json: {} })
    })
    await page.goto(`/link#${token}`)
    await expect(page.getByLabel('New password', { exact: true })).toBeVisible()
    expect(lookupHash).toBe('')
    await expect(page).toHaveURL(/\/link$/)
    await page.getByLabel('New password', { exact: true }).fill('synthetic long password')
    await page.getByRole('button', { name: 'Save password' }).click()
    if (purpose === 'reset') await expect(page.getByRole('status').filter({ hasText: 'Password saved' })).toBeVisible()
    else await expect(page).toHaveURL(/\/account$/)
  })
}
for (const status of [401, 409, 429]) {
  test(`link lookup failure ${status} uses fixed copy`, async ({ page }) => {
    await english(page)
    await page.route('**/api/v1/auth/link', route => route.fulfill({ status, json: failure(status === 429 ? 'rate_limited' : 'link_invalid') }))
    await page.goto(`/link#${token}`)
    await expect(page.getByRole('alert')).toContainText(status === 429 ? 'Too many' : 'expired or has already been used')
    await expect(page).toHaveURL(/\/link$/)
  })
}
