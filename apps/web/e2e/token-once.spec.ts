import { expect, test, type Page } from '@playwright/test'
import { openAccount, secret } from './account-fixture'
import { containsPrivateValue } from './privacy-state'

async function outsideState(page: Page) {
  const state = await page.evaluate(() => {
    const app = (document.querySelector('#__nuxt') as unknown as { __vue_app__?: { $nuxt?: { $pinia?: { state?: { value?: unknown } } } } })?.__vue_app__
    const pinia = app?.$nuxt?.$pinia?.state?.value
    if (!pinia) throw new Error('Nuxt Pinia state is unavailable')
    return { local: { ...localStorage }, session: { ...sessionStorage }, cookies: document.cookie, history: history.state, pinia, payload: document.querySelector('#__NUXT_DATA__')?.textContent, nuxt: (window as unknown as { __NUXT__?: unknown }).__NUXT__, url: location.href }
  })
  const collected = { state, cookies: await page.context().cookies() }
  expect(containsPrivateValue(collected, secret)).toBe(false)
  expect(JSON.stringify(collected)).not.toContain(JSON.stringify(secret).slice(1, -1))
}
async function create(page: Page) {
  await page.getByRole('button', { name: 'Create token', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Create token', exact: true })
  const bounds = await dialog.boundingBox()
  const viewport = page.viewportSize()!
  expect(bounds).not.toBeNull()
  expect(Math.abs(bounds!.x + bounds!.width / 2 - viewport.width / 2)).toBeLessThan(2)
  expect(Math.abs(bounds!.y + bounds!.height / 2 - viewport.height / 2)).toBeLessThan(2)
  await expect(page.getByLabel('Name')).toBeFocused()
  await page.getByLabel('Name').fill('test')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveJSProperty('open', true)
  await expect(page.locator('dialog:modal')).toHaveCount(1)
  await expect(page.getByRole('dialog').getByText(secret)).toBeVisible()
  expect(await page.getByText(secret).count()).toBe(1)
  await outsideState(page)
}
test('a token is shown once and cleared on close, navigation and reload', async ({ page }) => {
  await openAccount(page)
  await create(page)
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(page.getByText(secret)).toHaveCount(0)
  await outsideState(page)
  await create(page)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByText(secret)).toHaveCount(0)
  await outsideState(page)
  await create(page)
  await page.goBack()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByText(secret)).toHaveCount(0)
  await outsideState(page)
  await page.reload()
  await expect(page.getByText(secret)).toHaveCount(0)
  await page.getByRole('link', { name: 'Account', exact: true }).click()
  await expect(page.getByText(secret)).toHaveCount(0)
  await outsideState(page)
  await create(page)
  const response = await page.reload()
  expect(response?.status()).toBe(200)
  await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible()
  await expect(page.getByText(secret)).toHaveCount(0)
  await outsideState(page)
})
