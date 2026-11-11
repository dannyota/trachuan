import { expect, test, type Page } from '@playwright/test'
import { freshEmail, signIn } from './identity'

test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.10' } })

async function keyboard(page: Page) {
  const controls = page.locator('a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])')
  const expected = []
  for (const control of await controls.elementHandles()) {
    if (await control.evaluate(element => {
      const style = getComputedStyle(element)
      return style.visibility !== 'hidden' && element.getClientRects().length > 0 && !element.closest('[inert]')
    })) expected.push(control)
  }
  expect(expected.length).toBeGreaterThan(0)
  await page.locator('body').click({ position: { x: 1, y: 1 } })
  let previous = await page.evaluateHandle(() => document.activeElement)
  for (const control of expected) {
    let changed = false
    for (let press = 0; press < 5; press++) {
      await page.keyboard.press('Tab')
      changed = await previous.evaluate(element => element !== document.activeElement)
      if (changed) break
    }
    expect(changed, 'Tab must leave the previous control within five presses').toBe(true)
    expect(await control.evaluate(element => element === document.activeElement)).toBe(true)
    const indicator = await control.evaluate(element => {
      const style = getComputedStyle(element)
      return element.matches(':focus-visible') && ((style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0) || style.boxShadow !== 'none')
    })
    expect(indicator).toBe(true)
    await previous.dispose()
    previous = await page.evaluateHandle(() => document.activeElement)
  }
  await previous.dispose()
}

test('keyboard focus and validity contrast on the website', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('banner').getByRole('link', { name: 'Đăng nhập', exact: true })).toBeVisible()
  await keyboard(page)
  await page.goto('/login')
  await keyboard(page)
  await signIn(page, freshEmail())
  await expect(page.getByText('Phiên hiện tại')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Tạo mã truy cập' })).toBeVisible()
  await keyboard(page)
  await page.getByRole('button', { name: 'Tạo mã truy cập' }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  const dialog = page.getByRole('dialog')
  const dialogControls = dialog.locator('input, button')
  await expect(dialogControls.first()).toBeFocused()
  for (let index = 0; index < await dialogControls.count(); index++) {
    const control = dialogControls.nth(index)
    await expect(control).toBeFocused()
    expect(await control.evaluate(element => {
      const style = getComputedStyle(element)
      return element.matches(':focus-visible') && ((style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0) || style.boxShadow !== 'none')
    })).toBe(true)
    await page.keyboard.press('Tab')
  }
  await page.keyboard.press('Escape')
  await page.goto('/vn/search')
  await keyboard(page)
  await page.getByLabel('Câu hỏi tiếng Việt').fill('Văn bản thử nghiệm\nMẫu giả lập vườn thử nghiệm xanh')
  await page.getByRole('button', { name: 'Tra cứu', exact: true }).click()
  await expect(page.locator('.search-card').first()).toBeVisible()
  await keyboard(page)
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme })
    for (const ink of ['in-force', 'partial', 'not-yet', 'expired']) {
      const ratio = await page.locator(`.search-summary .ink-${ink}`).evaluate(element => {
        const rgb = (value: string) => value.match(/[\d.]+/g)!.slice(0, 3).map(Number)
        const luminance = (value: string) => rgb(value).map(channel => {
          const c = channel / 255
          return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
        }).reduce((sum, c, i) => sum + c * [0.2126, 0.7152, 0.0722][i]!, 0)
        const foreground = luminance(getComputedStyle(element).color)
        const background = luminance(getComputedStyle(document.body).backgroundColor)
        return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
      })
      expect(ratio, `${colorScheme} ${ink}`).toBeGreaterThanOrEqual(4.5)
      console.log(`contrast ${colorScheme} ${ink}: ${ratio.toFixed(2)}:1`)
    }
  }
  await page.locator('.search-card').first().getByRole('link', { name: 'Đọc văn bản' }).click()
  await expect(page.locator('[data-anchor="article-1"]')).toBeVisible()
  await expect(page.locator('.reader-rail').getByRole('link', { name: '2/2099/TT-TEST', exact: true })).toBeVisible()
  await keyboard(page)
})
