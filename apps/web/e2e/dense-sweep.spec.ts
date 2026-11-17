import { expect, test, type Page } from '@playwright/test'
import { clientAddress } from './client-address'

async function navigate(page: Page, path: string) {
  await page.evaluate(path => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push(path), path)
}

async function shapes(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  const buttons = page.locator('.search-header button:visible, .search-page button:visible, .reader-main button:visible')
  expect(await buttons.count()).toBeGreaterThan(0)
  for (const button of await buttons.all()) {
    const box = await button.boundingBox()
    expect(box!.height).toBeGreaterThanOrEqual(44)
    expect(parseFloat(await button.evaluate(el => getComputedStyle(el).borderTopLeftRadius))).toBeGreaterThanOrEqual(box!.height / 2)
  }
  for (const chip of await page.locator('.chip:visible, .reader-relation-chip:visible').all()) {
    const box = await chip.boundingBox()
    expect(box!.height).toBeGreaterThanOrEqual(24)
    expect(parseFloat(await chip.evaluate(el => getComputedStyle(el).borderTopLeftRadius))).toBeGreaterThanOrEqual(box!.height / 2)
  }
}

const documentNumber = '1234/2099/NĐ-CP-BTC-NHNN-TTCP'
const articleHeading = 'Quy định về trách nhiệm phối hợp và tổ chức thực hiện công tác quản lý, giám sát, bảo vệ thông tin và quyền lợi của người tham gia hoạt động trực tuyến'

for (const colorScheme of ['light', 'dark'] as const) {
  for (const width of [320, 360, 390, 768, 1024, 1366]) {
    test(`dense screens at ${width} in ${colorScheme}`, async ({ page }, info) => {
      await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
      await page.setViewportSize({ width: 1366, height: 900 })
      await page.emulateMedia({ colorScheme })
      await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', csrf_token: 'fixture' } }))
      let taxonomyFailed = true
      await page.route('**/api/v1/public/taxonomy**', route => taxonomyFailed ? route.fulfill({ status: 503 }) : route.fulfill({ json: { nodes: [{ code: '1', name: 'Fixture topic' }] } }))
      await page.route('**/api/v1/search', route => route.fulfill({ json: { release: '20990101', abstain: false, hits: [{ handle: 'vn:vbpl:1#dieu-1', doc: 'vn:vbpl:1', citation: 'Điều 1', text: 'Fixture law line one\nFixture law line two\nFixture law line three', complete: true, nodes: ['1'] }], nodes: { '1': { name: 'Fixture topic' } }, documents: { 'vn:vbpl:1': { number: documentNumber, type: 'Luật', issuer: 'Fixture issuer', title: 'Fixture law', status: { class: 'partial', current: true }, text_source: 'source_tree', text_complete: true, source_url: 'https://vbpl.vn/fixture' } } } }))
      await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, document: { number: documentNumber, title: 'Fixture law', status: { class: 'in_force', current: true }, text_source: 'none', text_complete: false, related: [{ dir: 'out', kind: 'legal_basis', number: '2/2099' }] }, units: [{ kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', heading: articleHeading, text: 'Fixture law' }] } } }))
      await page.goto('/')
      await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
      await page.setViewportSize({ width, height: 900 })
      await navigate(page, '/vn/search')
      await expect(page.locator('.search-idle')).toBeVisible()
      await shapes(page)
      if (width < 768) await page.locator('.search-mobile-chips button').first().click()
      const retry = page.locator('.search-filters:visible .node-picker .filter-error button').first()
      await expect(retry).toBeVisible()
      await expect(retry).toHaveCSS('text-decoration-line', 'underline')
      await expect(retry).toHaveCSS('text-underline-offset', '3px')
      await shapes(page)
      taxonomyFailed = false
      await retry.click()
      await expect(page.locator('.search-filters:visible').getByText('Fixture topic')).toBeVisible()
      await expect(page.locator('.search-filters:visible label:has(input[type=checkbox])').first()).toHaveCSS('gap', '8px')
      if (width < 768) await page.keyboard.press('Escape')
      await page.locator('#search-query').fill('Fixture')
      await page.locator('.search-box').evaluate(form => (form as HTMLFormElement).requestSubmit())
      await expect(page.locator('.result-card .stamp')).toBeVisible()
      await expect(page.locator('.result-card .chip')).toBeVisible()
      await expect(page.locator('.result-card')).toHaveCSS('border-top-left-radius', '16px')
      await expect(page.locator('.result-card-body h2')).toHaveCSS('font-family', /NotoSerif/)
      await expect(page.locator('.result-title')).toHaveCSS('font-family', /NotoSerif/)
      const source = page.locator('.result-source')
      await expect(source).toBeVisible()
      const [sourceBox, iconBox] = await Promise.all([source.boundingBox(), source.locator('svg').boundingBox()])
      expect(iconBox!.y + iconBox!.height).toBeLessThanOrEqual(sourceBox!.y + sourceBox!.height + 1)
      expect(iconBox!.x).toBeGreaterThan(sourceBox!.x)
      expect(sourceBox!.height).toBeGreaterThanOrEqual(44)
      const head = page.locator('.result-card-head')
      const statusBox = (await head.locator('.result-status').boundingBox())!
      const actionsBox = (await head.locator('.result-actions').boundingBox())!
      if (width < 1200) expect(actionsBox.y).toBeGreaterThanOrEqual(statusBox.y + statusBox.height)
      else {
        expect(actionsBox.y).toBeLessThan(statusBox.y + statusBox.height)
        expect(actionsBox.y + actionsBox.height).toBeGreaterThan(statusBox.y)
      }
      const resultNumber = head.locator('.result-number')
      await expect(resultNumber).toHaveText(documentNumber)
      await expect(resultNumber).not.toHaveCSS('text-overflow', 'ellipsis')
      expect(await resultNumber.evaluate(el => el.scrollWidth)).toBe(await resultNumber.evaluate(el => el.clientWidth))
      const fontSizes = await page.locator('.result-card').evaluate((card) => {
        const walker = document.createTreeWalker(card, NodeFilter.SHOW_TEXT)
        const sizes: number[] = []
        while (walker.nextNode()) {
          const node = walker.currentNode
          const parent = node.parentElement!
          if (node.textContent?.trim() && parent.getClientRects().length) sizes.push(parseFloat(getComputedStyle(parent).fontSize))
        }
        return sizes
      })
      expect(fontSizes.length).toBeGreaterThan(0)
      for (const size of fontSizes) expect(size).toBeGreaterThanOrEqual(12)
      await expect(page.locator('.result-card-footer')).toHaveCount(0)
      for (const area of await page.locator('.result-card-head, .result-card-body, .result-nodes').all()) await expect(area).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      for (const action of await head.locator('.result-actions a, .result-actions button').all()) {
        expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44)
        await expect(action).toHaveCSS('text-decoration-line', 'underline')
        await expect(action).toHaveCSS('font-size', '13px')
      }
      if (width === 1366) {
        const text = page.locator('.result-text')
        expect((await text.boundingBox())!.height / parseFloat(await text.evaluate(el => getComputedStyle(el).lineHeight))).toBeCloseTo(3, 0)
        expect((await page.locator('.result-card').boundingBox())!.height).toBeLessThanOrEqual(220)
      }
      await shapes(page)
      await page.locator('#search-query').focus()
      await page.keyboard.press('Tab')
      await expect(page.locator('.search-box button')).toBeFocused()
      await expect(page.locator('.search-box button')).toHaveCSS('outline-style', 'solid')
      await navigate(page, '/vn/documents/vn:vbpl:1#dieu-1')
      const target = page.locator('.reader-unit-target')
      await expect(target).toHaveCSS('background-color', colorScheme === 'light' ? 'rgb(241, 244, 250)' : 'rgb(19, 26, 46)')
      await expect(target).toHaveCSS('border-left-width', '3px')
      await expect(target).toHaveCSS('border-left-color', colorScheme === 'light' ? 'rgb(30, 58, 138)' : 'rgb(157, 180, 245)')
      await expect(page.locator('.reader-card').first()).toHaveCSS('border-top-left-radius', '16px')
      await expect(page.locator('.reader-relation-chip').first()).toBeVisible()
      const heading = page.locator('.reader-unit-target h2')
      const number = page.locator('.reader-identity-line .num')
      await expect(heading).toContainText(articleHeading)
      await expect(number).toHaveText(documentNumber)
      for (const [element, container, wraps] of [[heading, '.reader-content', width === 320], [number, '.reader-identity-inner', false]] as const) {
        const geometry = await element.evaluate((el, selector) => {
          const parent = el.closest(selector)!.getBoundingClientRect()
          const range = document.createRange()
          range.selectNodeContents(el)
          const lines = Array.from(range.getClientRects(), rect => ({ left: rect.left, right: rect.right, width: rect.width }))
          return { left: parent.left, right: parent.right, lines }
        }, container)
        expect(geometry.lines.length).toBeGreaterThanOrEqual(wraps ? 2 : 1)
        for (const line of geometry.lines) {
          expect(line.width).toBeGreaterThan(0)
          expect(line.left).toBeGreaterThanOrEqual(geometry.left - 1)
          expect(line.right).toBeLessThanOrEqual(geometry.right + 1)
        }
      }
      await shapes(page)
    })
  }
}
