import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'

test.beforeEach(async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
})

test('keeps the shell and retries the same failed search', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', csrf_token: 'fixture' } }))
  const bodies: string[] = []
  await page.route('**/api/v1/search', route => {
    bodies.push(route.request().postData() || '')
    return bodies.length === 1 ? route.fulfill({ status: 503 }) : route.fulfill({ json: { release: '20990101', hits: [], abstain: true } })
  })
  await page.goto('/')
  await page.waitForFunction(() => Boolean((document.querySelector('#__nuxt') as unknown as { __vue_app__?: unknown } | null)?.__vue_app__))
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search?q=ignored&node=1'))
  await expect(page).not.toHaveURL(/q=/)
  await page.locator('#search-query').fill('fixture query')
  await page.locator('.search-box').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await expect(page.getByText('Không thể tra cứu lúc này.')).toBeVisible()
  await expect(page.getByRole('banner')).toBeVisible()
  await expect(page.getByRole('contentinfo')).toBeVisible()
  await page.getByRole('alert').getByRole('button', { name: 'Thử lại' }).click()
  await expect(page.getByText('Không tìm thấy bằng chứng')).toBeVisible()
  expect(bodies).toHaveLength(2)
  expect(bodies[1]).toBe(bodies[0])
  await page.getByRole('button', { name: 'Bỏ bộ lọc' }).click()
  await expect.poll(() => bodies.length).toBe(3)
  expect(JSON.parse(bodies[2]).in_force_only).toBe(false)
})

test('hides sign in on auth pages and moves skip focus to main', async ({ page }) => {
  for (const path of ['/login', '/signup', '/reset']) {
    await page.goto(path)
    await expect(page.getByRole('banner').getByRole('link', { name: 'Đăng nhập' })).toHaveCount(0)
    await page.keyboard.press('Tab')
    await expect(page.getByRole('link', { name: 'Bỏ qua đến nội dung' })).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(page.locator('main')).toBeFocused()
  }
})

test('shows session failure inside the shell with retry', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', csrf_token: 'fixture' } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Tài khoản' })).toBeVisible()
  await page.unroute('**/api/v1/me')
  await page.route('**/api/v1/me', route => route.fulfill({ status: 503 }))
  await page.evaluate(() => {
    const app = (document.querySelector('#__nuxt') as unknown as { __vue_app__: { $nuxt: { $pinia: { state: { value: { session: { status: string } } } } }, config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__
    app.$nuxt.$pinia.state.value.session.status = 'unknown'
    return app.config.globalProperties.$router.push('/vn/search')
  })
  await expect(page.getByRole('banner')).toBeVisible()
  await expect(page.getByRole('contentinfo')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Thử lại' })).toBeVisible()
  await expect(page).toHaveTitle('Tạm thời gián đoạn · Tra Chuẩn')
})

test('moves skip focus to main on the fixed 404', async ({ page }) => {
  await page.goto('/missing-page')
  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: 'Bỏ qua đến nội dung' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.locator('main')).toBeFocused()
})

test('keeps the cleaned URL in router history', async ({ page, context }) => {
  await context.addCookies([{ name: 'account-fixture', value: '1', url: 'http://127.0.0.1:3031' }])
  await page.goto('/')
  const before = await page.evaluate(() => history.length)
  await page.goto('/vn/search?q=private&node=1')
  await expect(page).toHaveURL(/\/vn\/search\?node=1$/)
  expect(await page.evaluate(() => history.length)).toBe(before + 1)
  await page.getByRole('banner').getByRole('link', { name: 'Nguồn dữ liệu' }).click()
  await expect(page).toHaveURL(/\/sources$/)
  await page.goBack()
  await expect(page).toHaveURL(/\/vn\/search\?node=1$/)
  expect(await page.evaluate(() => history.length)).toBe(before + 2)
})

test('retries a failed later page with the original cursor and focuses results', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', csrf_token: 'fixture' } }))
  const bodies: Record<string, unknown>[] = []
  await page.route('**/api/v1/search', route => {
    bodies.push(route.request().postDataJSON())
    if (bodies.length === 2 || bodies.length === 3) return route.fulfill({ status: 503 })
    const first = bodies.length === 1
    return route.fulfill({ json: { release: '20990101', ...(first ? { next_cursor: 'cursor-a' } : {}), abstain: false, hits: [{ citation: first ? 'Điều 1' : 'Điều 2', complete: true, text: first ? 'First fixture' : 'Second fixture' }] } })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Tài khoản' })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search'))
  await page.locator('#search-query').fill('fixture query')
  await page.locator('.search-box').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await page.getByRole('button', { name: 'Xem thêm kết quả' }).click()
  const alert = page.getByRole('alert')
  const retry = alert.getByRole('button', { name: 'Thử lại' })
  await retry.focus()
  await page.keyboard.press('Enter')
  await expect(alert.getByRole('heading')).toBeFocused()
  await retry.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByText('Second fixture')).toBeVisible()
  await expect(page.getByText('First fixture')).toBeVisible()
  await expect(page.locator('.search-results h1')).toBeFocused()
  expect(bodies[2]).toEqual(bodies[1])
  expect(bodies[3]).toEqual(bodies[1])
})

test('clears filters on the submitted query and focuses the new results', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', csrf_token: 'fixture' } }))
  const bodies: Record<string, unknown>[] = []
  await page.route('**/api/v1/search', route => {
    bodies.push(route.request().postDataJSON())
    return route.fulfill({ json: { release: '20990101', hits: [], abstain: true } })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Tài khoản' })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search?node=1'))
  await page.locator('#search-query').fill('fixture query')
  await page.locator('.search-box').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await page.locator('#search-query').fill('')
  await page.getByRole('button', { name: 'Bỏ bộ lọc' }).focus()
  await page.keyboard.press('Enter')
  await expect.poll(() => bodies.length).toBe(2)
  expect(bodies[1]).toMatchObject({ query: 'fixture query', in_force_only: false })
  for (const key of ['nodes', 'doc_types', 'issuers', 'as_of']) expect(bodies[1]).not.toHaveProperty(key)
  await expect(page.locator('.search-results h1')).toBeFocused()
})
