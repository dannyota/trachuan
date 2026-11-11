import { randomUUID } from 'node:crypto'
import { expect, type Page } from '@playwright/test'

export function freshEmail() { return `${randomUUID()}@example.invalid` }

export async function authorize(page: Page, email: string, verified = true) {
  await expect(page.getByLabel('Email', { exact: true })).toBeVisible()
  await page.getByLabel('Email', { exact: true }).fill(email)
  if (!verified) await page.getByLabel('Email verified').uncheck()
  await page.getByRole('button', { name: 'Continue' }).click()
}

export async function signIn(page: Page, email: string) {
  await page.goto('/account')
  await authorize(page, email)
  await expect(page).toHaveURL(/\/account$/)
  await expect(page.getByText(email, { exact: true })).toBeVisible()
}
