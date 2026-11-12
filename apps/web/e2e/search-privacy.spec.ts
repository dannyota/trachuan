import { expect, test } from '@playwright/test'
import { containsPrivateValue } from './privacy-state'

const account = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' }

for (const query of ['private-query-for-search', 'private-query-for-search\nsecond-line']) {
  test(`search sends ${query.includes('\n') ? 'multiline' : 'single-line'} private values only in POST bodies`, async ({ page }) => {
    const requests: { method: string, body: unknown }[] = []
    const urls: string[] = []
    page.on('request', request => urls.push(request.url()))
    await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
    await page.route('**/api/v1/search', route => {
      requests.push({ method: route.request().method(), body: route.request().postDataJSON() })
      return route.fulfill({ json: { release: 'r1', next_cursor: requests.length === 1 ? 'cursor-1' : '', abstain: false, hits: [{ handle: 'vn:1#dieu-1', doc: 'vn:1', citation: 'Citation', text: 'Law text', complete: true }], documents: { 'vn:1': { status: { class: 'in_force', current: true }, text_source: 'source_tree', text_complete: true } } } })
    })
    await page.goto('/')
    await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
    await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search'))
    const input = page.locator('#search-query')
    await expect(input).toBeVisible()
    const before = await page.evaluate(() => history.length)
    await input.fill('   ')
    await page.locator('form').evaluate(form => (form as HTMLFormElement).requestSubmit())
    expect(requests).toHaveLength(0)
    await input.fill(query)
    await page.locator('input[type=date]').fill('2026-01-02')
    await page.locator('input[type=checkbox]').uncheck()
    await page.locator('form').evaluate(form => (form as HTMLFormElement).requestSubmit())
    await expect(page.getByText('Citation')).toBeVisible()
    await page.getByRole('button', { name: /show more|xem thêm/i }).click()
    await expect.poll(() => requests.length).toBe(2)
    expect(requests).toEqual([
      { method: 'POST', body: { country: 'vn', query, detail: 'full', limit: 20, as_of: '2026-01-02', in_force_only: false } },
      { method: 'POST', body: { country: 'vn', query, detail: 'full', limit: 20, as_of: '2026-01-02', in_force_only: false, release: 'r1', cursor: 'cursor-1' } },
    ])
    await expect(page.getByRole('button', { name: /show more|xem thêm/i })).toHaveCount(0)
    expect(containsPrivateValue(urls, query)).toBe(false)
    expect(containsPrivateValue(urls, '2026-01-02')).toBe(false)
    const browserState = await page.evaluate(() => ({ url: location.href, length: history.length, state: history.state, cookies: document.cookie, local: { ...localStorage }, session: { ...sessionStorage }, title: document.title }))
    const state = { browserState, databases: await page.evaluate(() => indexedDB.databases()), payload: await page.locator('#__NUXT_DATA__').textContent(), cookies: await page.context().cookies() }
    expect(containsPrivateValue(state, query)).toBe(false)
    expect(JSON.stringify(state)).not.toContain(JSON.stringify(query).slice(1, -1))
    expect(containsPrivateValue(browserState, '2026-01-02')).toBe(false)
    expect(await page.evaluate(() => history.length)).toBe(before)
    const response = await page.reload()
    expect(containsPrivateValue(await response!.text(), query)).toBe(false)
    expect(await response!.text()).not.toContain('Law text')
  })
}
