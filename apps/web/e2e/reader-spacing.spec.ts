import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'
import { checkControls } from './control-checks'

for (const colorScheme of ['light', 'dark'] as const) {
  for (const width of [320, 390, 768, 1024, 1366]) {
    test(`reader spacing at ${width} in ${colorScheme}`, async ({ page }, info) => {
      await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
      await page.setViewportSize({ width: 1366, height: 720 })
      await page.emulateMedia({ colorScheme })
      await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
      await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, document: { number: '1/2099', source_url: 'https://vbpl.vn/fixture', title: 'Test law', status: { class: 'in_force', current: true }, text_source: 'none', text_complete: false }, units: [
        { kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', text: '1. First clause.\n\n' },
        { kind: 'provision', anchor: 'dieu-1/khoan-2', label: 'Khoản 2', text: '2. Second clause.\n\na) Point text.\n' + 'Further legal text.\n'.repeat(35) },
      ] } } }))
      await page.goto('/')
      await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
      await page.setViewportSize({ width, height: 720 })
      await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/documents/vn:vbpl:1#dieu-1'))
      await expect(page.locator('.reader-source svg')).toBeVisible()
      await checkControls(page.locator('.reader-main'))
      const target = page.locator('.reader-unit-target')
      const heading = target.locator('.reader-target-heading')
      const paragraphs = target.locator('.law-text:visible')
      await expect(heading).toHaveCSS('background-color', colorScheme === 'light' ? 'rgb(241, 244, 250)' : 'rgb(19, 26, 46)')
      await expect(target).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(paragraphs.first()).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(page.locator('body')).toHaveCSS('background-color', colorScheme === 'light' ? 'rgb(255, 255, 255)' : 'rgb(12, 18, 34)')
      await expect(target).toHaveCSS('border-left-width', '3px')
      await expect(target).toHaveCSS('border-left-color', colorScheme === 'light' ? 'rgb(30, 58, 138)' : 'rgb(157, 180, 245)')
      const geometry = await target.evaluate(element => {
        const article = element.querySelector('.reader-unit')!.getBoundingClientRect()
        const band = element.getBoundingClientRect()
        const lines = Array.from(element.querySelectorAll<HTMLElement>('.law-text')).filter(line => getComputedStyle(line).display !== 'none')
        const first = lines[0]!.getBoundingClientRect()
        const second = lines[1]!.getBoundingClientRect()
        const point = lines[2]!.getBoundingClientRect()
        const last = lines.at(-1)!.getBoundingClientRect()
        return { article, band, gap: second.top - first.bottom, pointGap: point.top - second.bottom, margin: parseFloat(getComputedStyle(lines[0]!).marginBottom), pointIndent: point.left - second.left, lastBottom: last.bottom }
      })
      expect(geometry.band.top).toBeLessThanOrEqual(geometry.article.top)
      expect(geometry.band.bottom).toBeGreaterThanOrEqual(geometry.article.bottom)
      expect(geometry.band.bottom).toBeGreaterThanOrEqual(geometry.lastBottom)
      expect(geometry.gap).toBeCloseTo(geometry.margin, 1)
      expect(geometry.pointGap).toBeCloseTo(geometry.margin, 1)
      expect(geometry.pointIndent).toBe(0)
      await expect(paragraphs.nth(2)).toHaveCSS('padding-left', '24px')
      if (width === 320 || width === 390) {
        const columnWidth = await page.locator('.reader-content').evaluate(element => element.clientWidth - parseFloat(getComputedStyle(element).paddingLeft) - parseFloat(getComputedStyle(element).paddingRight))
        expect(columnWidth).toBe(width - 32)
      }
      await page.evaluate(() => window.scrollTo(0, 0))
      await expect(page.locator('.reader-top:visible')).toHaveCount(0)
      for (const position of ['middle', 'end'] as const) {
        await page.evaluate(position => window.scrollTo(0, position === 'middle' ? document.documentElement.scrollHeight / 2 : document.documentElement.scrollHeight), position)
        const button = page.locator('.reader-top:visible')
        await expect(button).toBeVisible()
        await expect(button).toHaveCount(1)
        expect(await button.evaluate((element, selector) => !!element.closest(selector), width >= 1024 ? '.reader-toc-desktop' : '.reader-compact')).toBe(true)
        const overlaps = await button.evaluate(element => {
          const button = element.getBoundingClientRect()
          const compact = document.querySelector<HTMLElement>('.reader-compact')!
          const contentTop = getComputedStyle(compact).display === 'none' ? 0 : compact.getBoundingClientRect().bottom
          return ['.reader-content', 'footer.site-footer'].some(selector => {
            const walker = document.createTreeWalker(document.querySelector(selector)!, NodeFilter.SHOW_TEXT)
            while (walker.nextNode()) {
              const range = document.createRange()
              range.selectNode(walker.currentNode)
              if (Array.from(range.getClientRects()).some(line => line.width > 0 && line.height > 0 && line.left < button.right && line.right > button.left && Math.max(line.top, selector === '.reader-content' ? contentTop : 0) < button.bottom && line.bottom > button.top)) return true
            }
            return false
          })
        })
        expect(overlaps).toBe(false)
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    })
  }
}
