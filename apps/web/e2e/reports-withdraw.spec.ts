import { expect, test } from '@playwright/test'
import { openAccount } from './account-fixture'

const report = { report_id: 1, kind: 'wrong_text', document_key: 'vn:vbpl:1', anchor: 'a1', document_number: '1/2020/QH', target: '', suggestion: {}, state: 'open', created_at: '2026-01-01T00:00:00Z', changed_at: '2026-01-01T00:00:00Z' }

test('withdraws only open reports with CSRF and refreshes', async ({ page }) => {
  let state = 'open'
  let csrf = ''
  await page.route('**/api/v1/me/reports', route => route.fulfill({ json: { reports: [{ ...report, state }, { ...report, report_id: 2, state: 'accepted' }] } }))
  await page.route('**/api/v1/me/reports/1', route => { csrf = route.request().headers()['x-csrf-token'] || ''; state = 'withdrawn'; return route.fulfill({ status: 204 }) })
  await openAccount(page, false)
  const section = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Reports', exact: true }) })
  await expect(section.getByRole('button', { name: 'Withdraw' })).toHaveCount(1)
  await section.getByRole('button', { name: 'Withdraw' }).click()
  await expect(section.getByRole('button', { name: 'Withdraw' })).toHaveCount(0)
  await expect(section.getByText('Withdrawn', { exact: true })).toBeVisible()
  expect(csrf).toBe('csrf')
})

test('refreshes after a report was already decided', async ({ page }) => {
  let state = 'open'
  await page.route('**/api/v1/me/reports', route => route.fulfill({ json: { reports: [{ ...report, state }] } }))
  await page.route('**/api/v1/me/reports/1', route => { state = 'accepted'; return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } }) })
  await openAccount(page, false)
  await page.getByRole('button', { name: 'Withdraw' }).click()
  await expect(page.getByRole('alert')).toContainText('already decided or withdrawn')
  await expect(page.getByText('Accepted', { exact: true })).toBeVisible()
})
