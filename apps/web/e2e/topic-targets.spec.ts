import { expect, test, type Page } from '@playwright/test'
import { hydrated } from './hydrated'
import { withPublicSite } from './public-fixture'

const child = { code: '1.1', name: 'Chủ đề con giả lập có tên rất dài để xuống dòng trên màn hình điện thoại hẹp', provisions: 1, documents: 1 }

const nameLinks = (page: Page) => page.locator('.topic-tree li > a').evaluateAll(links => links.map(link => ({ text: link.textContent!, display: getComputedStyle(link).display, height: link.getBoundingClientRect().height, width: link.getBoundingClientRect().width, row: link.closest('li')!.getBoundingClientRect().width })))

async function open(page: Page, origin: string, path: string, width: number) {
  await page.setViewportSize({ width, height: 900 })
  await page.goto(`${origin}${path}`)
  await hydrated(page)
  if (path === '/vn/topics') {
    await page.getByRole('button', { name: /Chủ đề mẫu/ }).click()
    await expect(page.getByRole('link', { name: /Chủ đề con giả lập/ })).toBeVisible()
  }
  await expect(page.locator('.topic-tree li > a').last()).toContainText('Chủ đề con')
}

test('topic names are 44 px tap targets below 768 px and unchanged from 768 px', async ({ page }) => {
  await page.route('**/api/v1/public/taxonomy?**', route => route.fulfill({ json: { nodes: [child] } }))
  await withPublicSite(true, async origin => {
    for (const path of ['/vn/topics', '/vn/topics/1']) {
      for (const width of [360, 390, 767]) {
        await open(page, origin, path, width)
        const links = await nameLinks(page)
        expect(links.length).toBeGreaterThan(0)
        for (const link of links) {
          expect(link.height).toBeGreaterThanOrEqual(44)
          if (link.text.length < 30) expect(link.width).toBeLessThan(link.row)
        }
      }
      await open(page, origin, path, 768)
      for (const link of await nameLinks(page)) {
        expect(link.display).toBe('inline')
        expect(link.height).toBeLessThan(30)
      }
    }
  }, [child])
})
