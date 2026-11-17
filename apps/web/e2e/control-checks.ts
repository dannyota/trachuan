import { expect, type Locator } from '@playwright/test'

export async function checkControls(area: Locator) {
  for (const link of await area.locator('a:not(.chip):not(.reader-relation-chip):visible').all()) {
    await expect(link).toHaveCSS('text-decoration-line', 'underline')
    await expect(link).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(link).toHaveCSS('border-top-width', '0px')
    expect((await link.boundingBox())!.height).toBeGreaterThanOrEqual(44)
  }
  for (const button of await area.locator('button:visible').all()) {
    await expect(button).toHaveCSS('text-decoration-line', 'none')
    expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44)
  }
}

export async function checkActionGeometry(card: Locator, width: number, search: boolean) {
  const page = card.page()
  const button = card.locator('.result-actions button')
  await button.scrollIntoViewIfNeeded()
  await page.evaluate(() => {
    let calls = 0
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => { if (calls++ > 0) throw new Error('refused') } } })
  })
  const geometry = async () => card.evaluate((element) => {
    const cardBox = element.getBoundingClientRect()
    const rect = (selector: string) => {
      const box = element.querySelector(selector)!.getBoundingClientRect()
      return { x: box.x - cardBox.x, y: box.y - cardBox.y, width: box.width, height: box.height }
    }
    return { link: rect('.result-actions > a'), button: rect('.result-actions button'), ...(element.querySelector('.result-rank') ? { rank: rect('.result-rank'), status: rect('.result-status') } : {}) }
  })
  const initial = await geometry()
  if (width < 360) {
    expect(initial.button.y).toBeGreaterThanOrEqual(initial.link.y + initial.link.height)
    expect(initial.button.x).toBeCloseTo(initial.link.x, 1)
  } else {
    expect(initial.button.y).toBeCloseTo(initial.link.y, 1)
    expect(initial.button.x).toBeGreaterThan(initial.link.x)
  }
  if (search && width < 1200) {
    expect(initial.rank!.y).toBeLessThan(initial.status!.y + initial.status!.height)
    expect(initial.rank!.y + initial.rank!.height).toBeGreaterThan(initial.status!.y)
  }
  await button.click()
  await expect(button).toHaveText('Đã sao chép')
  expect(await geometry()).toEqual(initial)
  await button.click()
  await expect(button).toHaveText('Chưa sao chép được')
  expect(await geometry()).toEqual(initial)
}
