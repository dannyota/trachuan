import { expect, test } from '@playwright/test'
import { withStatusSite } from './status-fixture'

test('connect wraps code and displays numbered steps at each width', async ({ page }) => {
  await page.goto('/connect')
  for (const [width, columns] of [[320, 1], [390, 1], [1023, 1], [1024, 2]] as const) {
    await page.setViewportSize({ width, height: 900 })
    const grid = page.locator('.connect-grid')
    expect(await grid.evaluate(el => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(columns)
    await expect(page.locator('.connect-card ol').first()).toHaveCSS('list-style-type', 'decimal')
    await expect(page.locator('.server-address pre')).toHaveCSS('font-size', '16px')
    for (const block of await page.locator('.page-code pre').all()) {
      await expect(block).toHaveCSS('white-space', 'pre-wrap')
      expect(await block.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('sources shows four coverage facts in a two by two grid on mobile', async ({ page }) => {
  await withStatusSite({ release: '20990101', documents: 12, provisions: 34, nodes: 2, newest_source_date: '2099-01-01', gaps: {} }, async origin => {
    await page.setViewportSize({ width: 390, height: 900 })
    await page.goto(`${origin}/sources`)
    const facts = page.locator('.coverage-stats > div')
    await expect(facts).toHaveCount(4)
    const positions = await facts.evaluateAll(elements => elements.map(el => ({ x: el.getBoundingClientRect().x, y: el.getBoundingClientRect().y })))
    expect(positions[0]!.y).toBe(positions[1]!.y)
    expect(positions[2]!.y).toBe(positions[3]!.y)
    expect(positions[2]!.y).toBeGreaterThan(positions[0]!.y)
    expect(positions[1]!.x).toBeGreaterThan(positions[0]!.x)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
})

test('sources fills the grid when the newest date is absent', async ({ page }) => {
  await withStatusSite({ release: '20990101', documents: 12, provisions: 34, nodes: 2, gaps: {} }, async origin => {
    for (const width of [390, 1024]) {
      await page.setViewportSize({ width, height: 900 })
      await page.goto(`${origin}/sources`)
      const grid = page.locator('.coverage-stats')
      const facts = grid.locator(':scope > div')
      await expect(facts).toHaveCount(3)
      const right = await grid.evaluate(el => el.getBoundingClientRect().right)
      const last = await facts.last().boundingBox()
      expect(last!.x + last!.width).toBeCloseTo(right, 0)
    }
  })
})
