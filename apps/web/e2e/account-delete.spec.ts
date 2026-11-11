import { expect, test } from '@playwright/test'
import { email, openAccount } from './account-fixture'

test('deletion requires the exact email', async ({ page }) => {
  await openAccount(page)
  await page.getByRole('button', { name: 'Delete account', exact: true }).click()
  const dialog = page.getByRole('dialog')
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
