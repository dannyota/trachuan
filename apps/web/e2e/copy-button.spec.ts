import { expect, test, type Page } from '@playwright/test'
import { hydrated } from './hydrated'

const widths = [360, 390, 1366]

async function states(page: Page, path: string, width: number) {
  await page.setViewportSize({ width, height: 900 })
  await page.goto(path)
  await hydrated(page)
  await page.evaluate(() => {
    const flag = { fail: false }
    Object.defineProperty(window, '__copyFlag', { value: flag })
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => { if (flag.fail) throw new Error('refused') } } })
  })
}

const measure = (button: ReturnType<Page['locator']>) => button.evaluate(element => {
  const icon = element.querySelector('svg')!.getBoundingClientRect()
  const text = element.querySelector('.copy-label > span')!.firstChild as Text
  const range = document.createRange()
  range.selectNodeContents(text)
  const box = element.getBoundingClientRect()
  return { gap: range.getBoundingClientRect().left - icon.right, box: [box.x, box.y, box.width, box.height] }
})

for (const path of ['/', '/connect']) {
  for (const width of widths) {
    test(`copy buttons on ${path} at ${width} px keep the label next to the icon in every state without resizing`, async ({ page }) => {
      await page.route('**/api/v1/me', route => route.fulfill({ status: 204 }))
      await states(page, path, width)
      const buttons = page.locator('main .ghost-button:has(.copy-label)')
      expect(await buttons.count()).toBeGreaterThan(0)
      for (const button of await buttons.all()) {
        await button.scrollIntoViewIfNeeded()
        const idle = await measure(button)
        expect(idle.gap).toBeLessThanOrEqual(8)
        await button.click()
        await expect(button).toHaveText('Đã sao chép')
        const copied = await measure(button)
        expect(copied.gap).toBeLessThanOrEqual(8)
        expect(copied.box).toEqual(idle.box)
        await page.evaluate(() => { (window as unknown as { __copyFlag: { fail: boolean } }).__copyFlag.fail = true })
        await button.click()
        await expect(button).toHaveText('Chưa sao chép được')
        const failed = await measure(button)
        expect(failed.gap).toBeLessThanOrEqual(8)
        expect(failed.box).toEqual(idle.box)
        await page.evaluate(() => { (window as unknown as { __copyFlag: { fail: boolean } }).__copyFlag.fail = false })
      }
    })
  }
}
