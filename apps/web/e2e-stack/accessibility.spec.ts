import { expect, test, type Locator, type Page, type Response } from '@playwright/test'
import type { components } from '@trachuan/schema/gen/openapi'
import { freshEmail, signIn } from './identity'

test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.10' } })

async function keyboard(page: Page) {
  const controls = page.locator('a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), details > summary:first-of-type, [tabindex]:not([tabindex="-1"])')
  const expected = []
  for (const control of await controls.elementHandles()) {
    if (await control.evaluate(node => {
      const element = node as HTMLElement
      const style = getComputedStyle(element)
      return style.visibility !== 'hidden' && element.getClientRects().length > 0 && !element.closest('[inert]')
    })) expected.push(control)
  }
  expect(expected.length).toBeGreaterThan(0)
  await page.evaluate(() => { document.body.tabIndex = -1; document.body.focus(); document.body.removeAttribute('tabindex') })
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
    const indicator = await control.evaluate(node => {
      const element = node as HTMLElement
      const style = getComputedStyle(element)
      return element.matches(':focus-visible') && ((style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0) || style.boxShadow !== 'none')
    })
    expect(indicator).toBe(true)
    await previous.dispose()
    previous = await page.evaluateHandle(() => document.activeElement)
  }
  await previous.dispose()
}

async function searchFilters(page: Page, taxonomy: Response, facets: Response) {
  const picker = page.locator('.search-rail .node-picker')
  if (taxonomy.ok()) {
    const body = await taxonomy.json() as components['schemas']['TaxonomyResult']
    if (body.nodes?.some(node => node.code && !node.code.includes('.'))) {
      await expect(picker.getByRole('checkbox').first()).toBeVisible()
    } else {
      await expect(picker.locator('ul > li')).toHaveCount(0)
    }
  } else {
    await expect(picker.getByRole('status')).toBeVisible()
  }
  if (facets.ok()) {
    const body = await facets.json() as components['schemas']['CorpusStatus']
    for (const [name, facets] of [['Loại văn bản', body.doc_types || []], ['Cơ quan ban hành', body.issuers || []]] as const) {
      const group = page.locator('.search-rail fieldset').filter({ has: page.locator('legend', { hasText: name }) })
      await expect(group.locator('.facet-option')).toHaveCount(Math.min(facets.length, 6))
      if (facets.length > 6) await group.getByRole('button', { name: 'Xem tất cả' }).click()
      await expect(group.locator('.facet-option')).toHaveCount(facets.length)
      for (const [index, facet] of facets.entries()) {
        const option = group.locator('.facet-option').nth(index)
        await expect(option).toContainText(facet.value || '')
        await expect(option.getByRole('checkbox')).toBeVisible()
      }
      if (facets.length > 6) await group.getByRole('button', { name: 'Thu gọn' }).click()
    }
  } else {
    await expect(page.locator('.search-rail .search-filters > [role="status"]')).toBeVisible()
  }
}

async function modalKeyboard(page: Page, dialog: Locator, controls: Locator[]) {
  await expect(controls[0]!).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
  await expect(controls[controls.length - 1]!).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(controls[0]!).toBeFocused()
  for (const control of controls) {
    await expect(control).toBeFocused()
    expect(await control.evaluate(element => {
      const style = getComputedStyle(element)
      return element.matches(':focus-visible') && ((style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0) || style.boxShadow !== 'none')
    })).toBe(true)
    await page.keyboard.press('Tab')
    expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
  }
}

test('keyboard focus and validity contrast on the website', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('banner').getByRole('link', { name: 'Đăng nhập', exact: true })).toBeVisible()
  await keyboard(page)
  await page.goto('/login')
  await keyboard(page)
  const accountEmail = freshEmail()
  await signIn(page, accountEmail)
  await expect(page.getByText('Phiên này')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Tạo mã mới' })).toBeVisible()
  await keyboard(page)
  const openToken = page.getByRole('button', { name: 'Tạo mã mới', exact: true })
  await openToken.click()
  const dialog = page.getByRole('dialog', { name: 'Tạo mã truy cập', exact: true })
  await expect(dialog).toBeVisible()
  const name = dialog.getByLabel('Tên', { exact: true })
  await name.fill('accessibility token')
  await modalKeyboard(page, dialog, [name, dialog.getByLabel('Số ngày'), dialog.getByRole('button', { name: 'Tạo', exact: true }), dialog.getByRole('button', { name: 'Đóng', exact: true })])
  await dialog.getByRole('button', { name: 'Tạo', exact: true }).focus()
  await page.keyboard.press('Enter')
  await expect(dialog.locator('.token-value')).toBeVisible()
  expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
  const copy = dialog.getByRole('button', { name: 'Sao chép', exact: true })
  if (!(await copy.evaluate(element => element === document.activeElement))) await page.keyboard.press('Tab')
  await modalKeyboard(page, dialog, [copy, dialog.getByRole('button', { name: 'Đóng', exact: true })])
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(openToken).toBeFocused()

  const openDeletion = page.getByRole('button', { name: 'Xóa tài khoản…', exact: true })
  await openDeletion.click()
  const deletion = page.getByRole('dialog', { name: 'Xóa tài khoản', exact: true })
  await expect(deletion).toBeVisible()
  const email = deletion.getByLabel('Email', { exact: true })
  await email.fill(accountEmail)
  await modalKeyboard(page, deletion, [email, deletion.getByRole('button', { name: 'Xóa tài khoản', exact: true }), deletion.getByRole('button', { name: 'Đóng', exact: true })])
  await page.keyboard.press('Escape')
  await expect(deletion).toHaveCount(0)
  await expect(openDeletion).toBeFocused()
  const taxonomyResponse = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/public/taxonomy')
  const facetsResponse = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/status')
  await page.goto('/vn/search')
  const [taxonomy, facets] = await Promise.all([taxonomyResponse, facetsResponse])
  await searchFilters(page, taxonomy, facets)
  await keyboard(page)
  await page.getByLabel('Câu hỏi tiếng Việt').fill('Văn bản thử nghiệm\nMẫu giả lập vườn thử nghiệm xanh')
  await page.getByRole('button', { name: 'Tra cứu', exact: true }).click()
  await expect(page.locator('.result-card').first()).toBeVisible()
  await searchFilters(page, taxonomy, facets)
  await keyboard(page)
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme })
    const legend = page.locator('.search-legend span')
    expect(await legend.count()).toBeGreaterThan(0)
    for (const ink of ['in-force', 'partial', 'not-yet', 'expired']) {
      const ratio = await page.locator('.search-legend').evaluate((legend, ink) => {
        const element = document.createElement('span')
        element.className = `ink-${ink}`
        legend.append(element)
        const rgb = (value: string) => value.match(/[\d.]+/g)!.slice(0, 3).map(Number)
        const luminance = (value: string) => rgb(value).map(channel => {
          const c = channel / 255
          return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
        }).reduce((sum, c, i) => sum + c * [0.2126, 0.7152, 0.0722][i]!, 0)
        const foreground = luminance(getComputedStyle(element).color)
        const background = luminance(getComputedStyle(document.body).backgroundColor)
        element.remove()
        return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
      }, ink)
      expect(ratio, `${colorScheme} ${ink}`).toBeGreaterThanOrEqual(4.5)
      console.log(`contrast ${colorScheme} ${ink}: ${ratio.toFixed(2)}:1`)
    }
  }
  await page.locator('.result-card').first().getByRole('link', { name: 'Đọc trong văn bản' }).click()
  await expect(page.locator('[data-anchor="dieu-1"]')).toBeVisible()
  await expect(page.locator('.reader-rail').getByRole('link', { name: '2/2099/TT-TEST', exact: true })).toBeVisible()
  await keyboard(page)
})
