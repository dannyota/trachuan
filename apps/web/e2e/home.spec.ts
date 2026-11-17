import { expect, test } from '@playwright/test'
import { withStatusSite } from './status-fixture'

const account = { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' }

test('signed-in home search submits its draft without putting the question in a URL', async ({ page }) => {
  const urls: string[] = []
  const query = 'private example question'
  page.on('request', request => urls.push(request.url()))
  await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
  await page.route('**/api/v1/status', route => route.fulfill({ status: 503 }))
  await page.route('**/api/v1/search', route => route.fulfill({ json: { abstain: false, hits: [] } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /tài khoản/i })).toBeVisible()
  await page.locator('#home-query').fill(query)
  await page.locator('.home-form-options label').click()
  await expect(page.locator('.home-form input[type=checkbox]')).not.toBeChecked()
  const searchRequest = page.waitForRequest(request => new URL(request.url()).pathname === '/api/v1/search')
  await page.locator('.home-form').getByRole('button', { name: 'Tra cứu' }).click()
  const request = await searchRequest
  expect(request.method()).toBe('POST')
  expect(request.postDataJSON()).toEqual({ country: 'vn', query, detail: 'full', limit: 20, in_force_only: false })
  await expect(page).toHaveURL(/\/vn\/search$/)
  await expect(page.locator('#search-query')).toHaveValue(query)
  expect(urls.every(url => !url.includes(encodeURIComponent(query)) && !url.includes(query))).toBe(true)
})

test('home keeps the sample current, public and usable on mobile', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/v1/me', route => route.fulfill({ status: 204 }))
  await page.goto('/')
  await expect(page.getByText('Bạn cần đăng nhập để tra cứu. Mình không ghi câu hỏi của bạn vào ổ đĩa hay nhật ký.', { exact: true })).toBeVisible()
  const sample = page.locator('.home-sample .result-card')
  await expect(sample.locator('mark')).toHaveCount(0)
  await expect(sample.locator('.result-rank')).toHaveCount(0)
  const href = await sample.locator('.result-card-footer a[href^="/vn/documents/"]').first().getAttribute('href')
  expect(href).not.toContain('release=')
  await expect(page.locator('.home-coverage .coverage-release')).toHaveCount(0)
  const address = page.locator('.home-address code')
  const copy = page.locator('.home-address button')
  await expect(address).toHaveCSS('font-size', '16px')
  expect(await address.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
  const addressBox = await address.boundingBox()
  const copyBox = await copy.boundingBox()
  expect(copyBox!.y).toBeGreaterThanOrEqual(addressBox!.y + addressBox!.height)
  expect(await copy.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

for (const width of [1280, 390]) {
  test(`home hero is inset at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/')
    const box = await page.locator('.home-hero').boundingBox()
    expect(box!.x).toBeGreaterThan(0)
    expect(box!.width).toBeLessThan(width)
    const connect = (await page.locator('.home-connect').boundingBox())!
    expect(connect.x).toBe(0)
    expect(connect.width).toBe(width)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

for (const width of [320, 390, 767, 768, 1023, 1024, 1280]) {
  test(`coverage values and code fit at ${width}px`, async ({ page }) => {
    await withStatusSite({ release: '20990101', documents: 1234567, provisions: 9876543, nodes: 2, newest_source_date: '2099-12-28', gaps: {} }, async origin => {
      await page.setViewportSize({ width, height: 900 })
      for (const language of ['vi', 'en']) {
        await page.setExtraHTTPHeaders({ 'x-trachuan-language': language })
        for (const path of ['/', '/sources']) {
          await page.goto(origin + path)
          const values = page.locator('.coverage-stats dd')
          await expect(values).toHaveCount(4)
          await expect(values.last()).toContainText('2099')
          for (const value of await values.all()) {
            expect(await value.evaluate(element => {
              const range = document.createRange()
              range.selectNodeContents(element)
              const text = range.getBoundingClientRect()
              const box = element.getBoundingClientRect()
              return text.width <= box.width + 0.5 && range.getClientRects().length === 1
            }), `${language} ${path}`).toBe(true)
          }
          expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
          if (path === '/sources') {
            const [cell, release] = await Promise.all([page.locator('.coverage-stats dd').first().evaluate(el => { const range = document.createRange(); range.selectNodeContents(el); return range.getBoundingClientRect().left }), page.locator('.coverage-release').evaluate(el => { const range = document.createRange(); range.selectNodeContents(el); return range.getBoundingClientRect().left })])
            expect(Math.abs(cell - release), `${language} ${path}`).toBeLessThanOrEqual(1)
          }
        }
      }
      await page.goto(origin)
      const command = page.locator('.home-connect-card pre')
      await expect(command).toHaveCSS('white-space', 'pre-wrap')
      expect(await command.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
    })
  })
}
