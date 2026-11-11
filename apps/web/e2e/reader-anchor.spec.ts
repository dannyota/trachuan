import { expect, test } from '@playwright/test'

const account = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' }
const anchor = 'dieu-"]-[x'

test('finds a quoted anchor on a later page and leaves unknown anchors at the top', async ({ page }) => {
  await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
  await page.route('**/api/v1/documents/*', route => {
    const next = new URL(route.request().url()).searchParams.get('next')
    return route.fulfill({ json: next
      ? { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', anchor, text: 'Target provision' }, { kind: 'provision', anchor: 'after', text: 'Following provision\n'.repeat(200) }] } }
      : { release: '20261111_1', item: { complete: false, next: 'page2', units: [{ kind: 'provision', anchor: 'first', text: 'First provision\n'.repeat(300) }] } } })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  const navigate = (path: string) => page.evaluate(path => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push(path), path)
  await navigate(`/vn/documents/vn:vbpl:1#${encodeURIComponent(anchor)}`)
  const target = page.getByText('Target provision')
  await expect(target).toBeVisible()
  await expect.poll(() => target.evaluate(el => Math.abs(el.getBoundingClientRect().top))).toBeLessThan(150)
  await navigate('/vn/documents/vn:vbpl:1#unknown')
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0)
})
