import { expect, test } from '@playwright/test'
import { email, openAccount } from './account-fixture'

test('deletion requires the exact email', async ({ page }) => {
  await openAccount(page)
  await page.getByRole('button', { name: 'Delete account…', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Delete account', exact: true })
  const bounds = await dialog.boundingBox()
  const viewport = page.viewportSize()!
  expect(bounds).not.toBeNull()
  expect(Math.abs(bounds!.x + bounds!.width / 2 - viewport.width / 2)).toBeLessThan(2)
  expect(Math.abs(bounds!.y + bounds!.height / 2 - viewport.height / 2)).toBeLessThan(2)
  await expect(page.locator('dialog:modal')).toHaveCount(1)
  await expect(dialog.getByLabel('Email')).toBeFocused()
  const confirm = dialog.getByRole('button', { name: 'Delete account', exact: true })
  await expect(confirm).toBeDisabled()
  for (const value of [email.toUpperCase(), ` ${email}`, `${email} `]) {
    await dialog.getByLabel('Email').fill(value)
    await expect(confirm).toBeDisabled()
  }
  await dialog.getByLabel('Email').fill(email)
  await expect(confirm).toBeEnabled()
})

test('account spacing applies once at desktop and mobile widths', async ({ page }) => {
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await openAccount(page)
    const main = await page.locator('main').boundingBox()
    const heading = await page.getByRole('heading', { name: 'Account', exact: true }).boundingBox()
    const card = await page.locator('.account-card').first().boundingBox()
    expect(main).not.toBeNull()
    expect(heading).not.toBeNull()
    expect(card).not.toBeNull()
    expect(Math.abs(main!.x)).toBeLessThan(2)
    expect(Math.abs(main!.width - width)).toBeLessThan(2)
    expect(await page.locator('main').evaluate(element => getComputedStyle(element).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)')
    expect(Math.abs(heading!.y - main!.y - (width < 768 ? 24 : 40))).toBeLessThan(2)
    expect(Math.abs(card!.x - (width < 768 ? 16 : 184))).toBeLessThan(2)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  }
})
