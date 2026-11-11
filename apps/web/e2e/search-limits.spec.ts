import { expect, test } from '@playwright/test'

test('shows the retry delay on rate limiting', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' } }))
  await page.route('**/api/v1/search', route => route.fulfill({ status: 429, headers: { 'Retry-After': '17' }, json: { error: { code: 'rate_limited', message: 'rate limited' } } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search'))
  await page.locator('#search-query').fill('query')
  await page.locator('form').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await expect(page.getByRole('alert')).toContainText('17')
})

test('restarts an expired pinned search without the release', async ({ page }) => {
  const bodies: { release?: string, limit: number }[] = []
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' } }))
  await page.route('**/api/v1/search', route => {
    const body = route.request().postDataJSON()
    bodies.push(body)
    if (body.release) return route.fulfill({ status: 409, json: { error: { code: 'release_expired', message: 'expired' } } })
    return route.fulfill({ json: { release: bodies.length === 1 ? 'r1' : 'r2', abstain: false, hits: [{ handle: 'vn:vbpl:1', doc: 'vn:vbpl:1', citation: 'Citation', complete: true }], documents: { 'vn:vbpl:1': { status: { current: true }, text_source: 'source_tree', text_complete: true } } } })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search'))
  await page.locator('#search-query').fill('query')
  await page.locator('form').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await page.getByRole('button', { name: /show more|xem thêm/i }).click()
  await expect.poll(() => bodies.length).toBe(3)
  expect(bodies.map(({ limit, release }) => ({ limit, release }))).toEqual([{ limit: 20, release: undefined }, { limit: 50, release: 'r1' }, { limit: 50, release: undefined }])
  await expect(page.getByRole('link', { name: /read document|đọc văn bản/i })).toHaveAttribute('href', /release=r2/)
})

test('renders compact and full evidence as text and shows gaps and abstention', async ({ page }) => {
  let call = 0
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' } }))
  await page.route('**/api/v1/search', route => {
    call++
    const base = { release: 'r1', documents: { 'vn:vbpl:1': { status: { class: 'future_status', current: false }, text_source: 'source_tree', text_complete: true } }, gaps: [{ kind: 'missing', message: 'Gap detail' }] }
    const hit = { handle: 'vn:vbpl:1#dieu-1', doc: 'vn:vbpl:1', citation: 'Citation', complete: true }
    const result = call === 3 ? { ...base, abstain: true, hits: [hit] } : { ...base, abstain: false, hits: [{ ...hit, ...(call === 1 ? { snippet: '<compact>' } : { text: '<full>' }) }] }
    return route.fulfill({ json: result })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search'))
  await page.locator('#search-query').fill('query')
  const submit = () => page.locator('form').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await submit()
  await expect(page.locator('.law-text')).toHaveText('<compact>')
  await expect(page.locator('.stamp')).toHaveClass(/ink-neutral/)
  await expect(page.locator('.stamp')).toHaveText('future_status')
  await expect(page.getByText('Gap detail')).toBeVisible()
  await submit()
  await expect(page.locator('.law-text')).toHaveText('<full>')
  await submit()
  await expect(page.locator('.search-card')).toHaveCount(0)
  await expect(page.getByText('Gap detail')).toBeVisible()
  await expect(page.getByText(/no evidence found|không tìm thấy bằng chứng/i)).toBeVisible()
})
