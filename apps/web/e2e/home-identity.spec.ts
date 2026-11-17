import { expect, test } from '@playwright/test'
import { clientAddress } from './client-address'
import { checkControls } from './control-checks'

test.beforeEach(async ({ page }, info) => { await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) }) })

for (const width of [320, 360, 390, 768, 1024, 1366]) {
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
        if (await pill.isVisible()) {
          expect((await pill.boundingBox())!.height).toBeGreaterThanOrEqual(await pill.evaluate(el => el.matches('.chip') || el.closest('.result-card') ? 44 : 52))
        }
      }
      const citation = page.locator('.home-citation')
      await expect(citation).toHaveText('Điều 19 Luật 91/2025/QH15')
      await expect(citation).toBeVisible()
      const citationBox = (await citation.boundingBox())!
      expect(citationBox.x).toBeGreaterThanOrEqual(0)
      expect(citationBox.x + citationBox.width).toBeLessThanOrEqual(width)
      await expect(page.locator('.home-sample .result-title')).toHaveCSS('font-family', /NotoSerif/)
      await expect(page.locator('.home-sample .result-card-body h2')).toHaveCSS('font-family', /NotoSerif/)
      const connect = hero.getByRole('link', { name: 'Xem cách kết nối' })
      await expect(connect).toHaveAttribute('href', '/connect')
      await expect(connect).toHaveCSS('border-top-color', 'rgb(255, 255, 255)')
      await expect(connect).toHaveCSS('border-top-width', '2px')
      await expect(connect).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await checkControls(page.locator('.home-sample .result-card'))
      const [actionsBox, captionBox] = await Promise.all([page.locator('.home-sample .result-actions').boundingBox(), page.locator('.home-sample .result-source').boundingBox()])
      expect(captionBox!.y).toBeGreaterThanOrEqual(actionsBox!.y + actionsBox!.height - 1)
      const readLink = page.locator('.home-sample .result-actions > a').first()
      await expect(readLink).toHaveCSS('text-decoration-line', 'underline')
      await expect(readLink).toHaveCSS('text-decoration-thickness', '1px')
      await expect(readLink).toHaveCSS('text-underline-offset', '3px')
      const checkbox = hero.locator('input[type=checkbox]')
      const box = hero.locator('.home-checkbox-box')
      const check = box.locator('svg')
      await expect(checkbox).toBeChecked()
      await expect(box).toHaveCSS('border-top-color', 'rgb(255, 255, 255)')
      await expect(check).toBeVisible()
      await expect(check).toHaveCSS('stroke', 'rgb(255, 255, 255)')
      await hero.locator('.home-form-options label').click()
      await expect(checkbox).not.toBeChecked()
      await expect(box).toHaveCSS('border-top-color', 'rgb(255, 255, 255)')
      await expect(check).toBeHidden()
      await expect(check).toHaveCSS('stroke', 'rgb(255, 255, 255)')
      await hero.locator('.home-form-options label').click()
      await expect(checkbox).toBeChecked()
      await checkbox.focus()
      await page.keyboard.press('Shift+Tab')
      await page.keyboard.press('Tab')
      await expect(checkbox).toBeFocused()
      await expect(box).toHaveCSS('outline-style', 'solid')
      await expect(box).toHaveCSS('outline-color', 'rgb(255, 255, 255)')
      await expect(box).toHaveCSS('outline-width', '3px')
      await expect(box).toHaveCSS('outline-offset', '3px')
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
    await expect(page.locator('body')).toHaveCSS('font-family', 'PlusJakartaSans, sans-serif')
    await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  }
})
