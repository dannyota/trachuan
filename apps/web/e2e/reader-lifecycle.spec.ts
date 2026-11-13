import { expect, test, type Page } from '@playwright/test'

const account = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' }
async function navigate(page: Page, path: string) {
  await page.evaluate(path => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push(path), path)
}
async function start(page: Page) {
  await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
}

test('automatically loads consecutive outline-only pages', async ({ page }) => {
  let requests = 0
  await page.route('**/api/v1/documents/*', route => {
    requests++
    return route.fulfill({ json: requests <= 3
      ? { release: '20261111_1', item: { complete: false, next: `p${requests}`, units: [{ kind: 'outline', anchor: `a${requests}`, label: `Outline ${requests}` }] } }
      : { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', text: 'Final provision' }] } } })
  })
  await start(page)
  await navigate(page, '/vn/documents/vn:vbpl:1')
  await expect(page.getByText('Final provision')).toBeVisible()
  expect(requests).toBe(4)
})

test('leaving a reader cancels its delayed request and prevents later scrolling', async ({ page }) => {
  let requests = 0
  let releaseResponse!: () => void
  let requestArrived!: () => void
  let responseHandled!: () => void
  const arrived = new Promise<void>(resolve => { requestArrived = resolve })
  const released = new Promise<void>(resolve => { releaseResponse = resolve })
  const handled = new Promise<void>(resolve => { responseHandled = resolve })
  await page.route('**/api/v1/documents/*', async route => {
    requests++
    requestArrived()
    await released
    await route.fulfill({ json: { release: '20261111_1', item: { complete: false, next: 'next', units: [] } } }).catch(() => {})
    responseHandled()
  })
  await start(page)
  await navigate(page, '/vn/documents/vn:vbpl:1#missing')
  await arrived
  await navigate(page, '/connect')
  await expect(page.getByRole('heading', { name: /connect|kết nối/i, level: 1 })).toBeVisible()
  await page.evaluate(() => document.fonts.ready)
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await page.evaluate(() => {
    const spacer = document.createElement('div')
    spacer.style.height = '3000px'
    document.body.append(spacer)
    window.scrollTo(0, 500)
  })
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(500)
  releaseResponse()
  await handled
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(500)
  expect(requests).toBe(1)
})
