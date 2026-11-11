import { expect, type Page } from '@playwright/test'

export const secret = 'trp_fixture_secret_only_once'
export const email = 'test@example.invalid'
export async function openAccount(page: Page, stubReports = true) {
  await page.context().addCookies([{ name: 'lang', value: 'en', url: 'http://127.0.0.1:3031' }, { name: 'account-fixture', value: '1', url: 'http://127.0.0.1:3031' }])
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email, created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf' } }))
  if (stubReports) await page.route('**/api/v1/me/reports', route => route.fulfill({ json: { reports: [] } }))
  await page.route('**/api/v1/me/sessions', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/me/grants', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/me/tokens', route => route.request().method() === 'POST'
    ? route.fulfill({ status: 201, json: { id: 1, name: 'test', created_at: '2026-01-01T00:00:00Z', expires_at: '2026-04-01T00:00:00Z', last_used_at: null, revoked_at: null, token: secret } })
    : route.fulfill({ json: [] }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Account', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Account', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible()
}
