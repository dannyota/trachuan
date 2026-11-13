import { expect, test } from '@playwright/test'

const account = { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' }

test('signed-in home search submits its draft without putting the question in a URL', async ({ page }) => {
  const urls: string[] = []
  const query = 'private example question'
  page.on('request', request => urls.push(request.url()))
  await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
  await page.route('**/api/v1/status', route => route.fulfill({ status: 503 }))
  await page.route('**/api/v1/search', route => {
    expect(route.request().postDataJSON()).toMatchObject({ query, in_force_only: false })
    return route.fulfill({ json: { abstain: false, hits: [] } })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: /tài khoản/i })).toBeVisible()
  await page.locator('#home-query').fill(query)
  await page.locator('.home-form input[type=checkbox]').uncheck()
  await page.locator('.home-form').getByRole('button', { name: 'Tra cứu' }).click()
  await expect(page).toHaveURL(/\/vn\/search$/)
  await expect(page.locator('#search-query')).toHaveValue(query)
  expect(urls.every(url => !url.includes(encodeURIComponent(query)) && !url.includes(query))).toBe(true)
})

test('home keeps the sample current, public and usable on mobile', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/v1/me', route => route.fulfill({ status: 401 }))
  await page.goto('/')
  await expect(page.getByText('Cần đăng nhập Google để tra cứu.')).toBeVisible()
  const sample = page.locator('.home-sample .result-card')
  await expect(sample.locator('mark')).toHaveCount(0)
  await expect(sample.locator('.result-rank')).toHaveCount(0)
  const href = await sample.locator('.result-card-footer a[href^="/vn/documents/"]').first().getAttribute('href')
  expect(href).not.toContain('release=')
  await expect(page.locator('.home-coverage .coverage-release')).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
