import { expect, test } from '@playwright/test'

for (const width of [360, 390, 768, 1024, 1366]) {
  for (const colorScheme of ['light', 'dark'] as const) {
    test(`home identity fits at ${width}px in ${colorScheme}`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 })
      await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' })
      await page.goto('/')
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      const hero = page.locator('.home-hero')
      await expect(hero).toHaveCSS('border-radius', width >= 1024 ? '44px' : width >= 768 ? '36px' : '28px')
      const heroBox = (await hero.boundingBox())!
      const sample = (await page.locator('.home-sample .result-card').boundingBox())!
      expect(sample.x).toBeGreaterThanOrEqual(0)
      expect(sample.x + sample.width).toBeLessThanOrEqual(width)
      expect(sample.y).toBeLessThan(heroBox.y + heroBox.height)
      expect(sample.y + sample.height).toBeGreaterThan(heroBox.y + heroBox.height)
      for (const pill of await page.locator('.home-page .btn, .home-page button, .home-page .chip').all()) {
        if (await pill.isVisible() && !await pill.evaluate(el => !!el.closest('.result-card'))) {
          expect((await pill.boundingBox())!.height).toBeGreaterThanOrEqual(await pill.evaluate(el => el.matches('.chip') ? 44 : 52))
        }
      }
      const input = page.locator('#home-query')
      expect(await input.evaluate(el => getComputedStyle(el).backgroundColor)).toBe(await page.evaluate(() => {
        const probe = document.createElement('div')
        probe.style.backgroundColor = getComputedStyle(document.documentElement).getPropertyValue('--color-hero-field')
        document.body.append(probe)
        const color = getComputedStyle(probe).backgroundColor
        probe.remove()
        return color
      }))
      await input.focus()
      await expect(input).toHaveCSS('outline-color', 'rgb(255, 255, 255)')
      await expect(page.locator('.home-suggestions button').first()).toHaveCSS('transition-duration', '0s')
    })
  }
}

test('other routes retain their chrome', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  for (const path of ['/sources', '/connect']) {
    await page.goto(path)
    expect(await page.locator('html').getAttribute('data-page')).toBeNull()
    await expect(page.locator('body')).toHaveCSS('font-family', /BeVietnam/)
    await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  }
})
