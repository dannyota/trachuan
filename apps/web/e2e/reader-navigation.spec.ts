import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'

test('navigates a long reader on desktop and mobile', async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'x-forwarded-for': clientAddress(info) })
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'synthetic@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
  const articles = Array.from({ length: 300 }, (_, i) => ({ kind: 'provision', anchor: `dieu-${i + 1}`, label: `Điều ${i + 1}`, heading: `Synthetic ${i + 1}`, text: 'Synthetic text. '.repeat(100) }))
  const outline = articles.map(unit => ({ ...unit, kind: 'outline', text: undefined }))
  await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, document: { number: '1/2099', title: 'Synthetic law', status: { class: 'in_force' }, text_source: 'none', text_complete: false }, units: [...outline, ...articles] } } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/documents/vn:vbpl:1'))
  await page.setViewportSize({ width: 1280, height: 720 })
  await expect(page.locator('.reader-toc-desktop')).toHaveCSS('position', 'sticky')
  await page.locator('[data-anchor="dieu-5"]').evaluate(element => element.scrollIntoView())
  await expect(page.locator('.reader-toc-desktop [aria-current="location"]').last()).toContainText('Điều 5')
  await expect(page.getByRole('button', { name: /lên đầu trang/i })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  const open = page.getByRole('button', { name: 'Mục lục' })
  await open.click()
  await page.keyboard.press('Escape')
  await expect(open).toBeFocused()
  await open.click()
  await page.getByRole('dialog').getByRole('link', { name: /Điều 10\./ }).click()
  await expect(page.getByRole('dialog')).toBeHidden()
  const target = page.locator('[data-anchor="dieu-10"]')
  await expect(target).toBeFocused()
  await expect.poll(() => target.evaluate(element => Math.abs(element.getBoundingClientRect().top - document.querySelector('.reader-compact')!.getBoundingClientRect().bottom))).toBeLessThan(2)
  await page.getByRole('button', { name: /lên đầu trang/i }).click()
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0)
})

test('tracks long articles when scrolling down and back up', async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'synthetic@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
  const outline = [1, 2].map(i => ({ kind: 'outline', anchor: `dieu-${i}`, label: `Điều ${i}` }))
  const articles = [1, 2].map(i => ({ kind: 'provision', anchor: `dieu-${i}`, label: `Điều ${i}`, text: 'Synthetic text.\n'.repeat(150) }))
  await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, units: [...outline, ...articles] } } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/documents/vn:vbpl:1#dieu-2'))
  await expect(page.locator('.reader-toc-desktop [aria-current="location"]')).toHaveText('Điều 2')
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  const scroll = (distance: number) => page.evaluate(async distance => {
    for (let i = 0; i < 30; i++) {
      window.scrollBy(0, distance)
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
    }
  }, distance)
  await scroll(-5)
  await expect(page.locator('.reader-toc-desktop [aria-current="location"]')).toHaveText('Điều 1')
  await scroll(5)
  await expect(page.locator('.reader-toc-desktop [aria-current="location"]')).toHaveText('Điều 2')
})

test('keeps a late anchored entry visible in a tall viewport', async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
  await page.setViewportSize({ width: 1280, height: 1440 })
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'synthetic@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
  const outline = Array.from({ length: 300 }, (_, i) => ({ kind: 'outline', anchor: `dieu-${i + 1}`, label: `Điều ${i + 1}` }))
  const articles = outline.map(unit => ({ ...unit, kind: 'provision', text: 'Synthetic text. '.repeat(60) }))
  await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, units: [...outline, ...articles] } } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/documents/vn:vbpl:1#dieu-290'))
  const selected = page.locator('.reader-toc-desktop [aria-current="location"]')
  await expect(selected).toHaveText('Điều 290')
  await expect.poll(() => selected.evaluate(link => {
    const item = link.getBoundingClientRect()
    const box = link.closest('.reader-toc-desktop')!.getBoundingClientRect()
    return item.top >= box.top && item.bottom <= box.bottom
  })).toBe(true)
})

test('tracks large jumps against heading positions and clears at the top', async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'synthetic@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
  const outline = Array.from({ length: 300 }, (_, i) => ({ kind: 'outline', anchor: `dieu-${i + 1}`, label: `Điều ${i + 1}` }))
  const articles = outline.map(unit => ({ ...unit, kind: 'provision', text: 'Synthetic text. '.repeat(100) }))
  await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, units: [...outline, ...articles] } } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/documents/vn:vbpl:1#dieu-150'))
  await expect(page.locator('[data-spy-anchor="dieu-150"]')).toBeVisible()
  await expect(page.locator('.reader-toc-desktop [data-toc-anchor="dieu-150"]')).toHaveAttribute('aria-current', 'location')
  const check = () => expect.poll(() => page.evaluate(() => {
    const edge = document.querySelector('.site-header')!.getBoundingClientRect().bottom
    const headings = Array.from(document.querySelectorAll<HTMLElement>('.reader-content [data-spy]'))
    const truth = headings.filter(heading => heading.getBoundingClientRect().top <= edge + 1).at(-1)?.dataset.spyAnchor || ''
    const selected = document.querySelector<HTMLElement>('.reader-toc-desktop [aria-current="location"]')?.dataset.tocAnchor || ''
    return { truth, selected, matches: truth === selected }
  })).toHaveProperty('matches', true)
  await check()
  for (const distance of [3000, 3000, -1500]) {
    await page.evaluate(distance => window.scrollBy(0, distance), distance)
    await check()
  }
  await page.evaluate(() => window.scrollTo(0, 0))
  await check()
  await expect(page.locator('.reader-toc-desktop [aria-current="location"]')).toHaveCount(0)
})

test('tracks direct jumps between the interiors of long articles', async ({ page }, info) => {
  await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'synthetic@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
  const outline = [1, 2, 3].map(i => ({ kind: 'outline', anchor: `dieu-${i}`, label: `Điều ${i}` }))
  const articles = outline.map(unit => ({ ...unit, kind: 'provision', text: 'Synthetic text.\n'.repeat(120) }))
  await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, units: [...outline, ...articles] } } }))
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/documents/vn:vbpl:1'))
  await expect(page.locator('[data-anchor="dieu-3"]')).toBeVisible()
  for (const i of [1, 2, 1, 3, 2]) {
    const article = page.locator(`[data-anchor="dieu-${i}"]`)
    expect((await article.boundingBox())!.height).toBeGreaterThan(3000)
    await article.evaluate(element => window.scrollTo(0, window.scrollY + element.getBoundingClientRect().top + 1400))
    const geometry = await page.evaluate(() => {
      const edge = document.querySelector('.site-header')!.getBoundingClientRect().bottom + 1
      const headings = Array.from(document.querySelectorAll<HTMLElement>('.reader-content [data-spy]'))
      return {
        visible: headings.some(heading => { const rect = heading.getBoundingClientRect(); return rect.bottom >= edge && rect.top <= window.innerHeight }),
        truth: headings.filter(heading => heading.getBoundingClientRect().top <= edge).at(-1)?.dataset.spyAnchor || '',
      }
    })
    expect(geometry.visible).toBe(false)
    expect(geometry.truth).toBe(`dieu-${i}`)
    await expect(page.locator('.reader-toc-desktop [aria-current="location"]')).toHaveAttribute('data-toc-anchor', geometry.truth)
  }
})
