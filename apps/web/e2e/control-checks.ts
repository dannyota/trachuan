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
