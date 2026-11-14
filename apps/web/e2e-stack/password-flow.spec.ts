import { createHmac } from 'node:crypto'
import { readdir, readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'
import { freshEmail } from './identity'

const password = 'synthetic registration passphrase 8472'
const replacement = 'synthetic replacement passphrase 9351'
test.use({ extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.31' } })

async function mailLink(email: string, purpose: 'registration' | 'reset') {
  const directory = process.env.SERVER_MAIL_CAPTURE_DIR
  if (!directory) throw new Error('SERVER_MAIL_CAPTURE_DIR is required')
  let link = ''
  await expect.poll(async () => {
    for (const file of await readdir(directory)) {
      if (!file.startsWith('message-') || !file.endsWith('.json')) continue
      const mail = JSON.parse(await readFile(join(directory, file), 'utf8')) as { to: string, subject: string, text: string }
      if (mail.to !== email || !mail.subject.includes(purpose === 'reset' ? 'Reset password' : 'Verify email')) continue
      link = mail.text.match(/https?:\/\/[^\s]+\/link#[A-Za-z0-9_-]+/)?.[0] || ''
      if (link) return true
    }
    return false
  }, { timeout: 20000 }).toBe(true)
  return link
}
function totp(secret: string, offset = 0) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  const bits = [...secret].map(char => alphabet.indexOf(char).toString(2).padStart(5, '0')).join('')
  const key = Buffer.from(bits.match(/.{8}/g)!.map(byte => parseInt(byte, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000) + offset))
  const digest = createHmac('sha1', key).update(counter).digest()
  const index = digest[digest.length - 1]! & 15
  return ((digest.readUInt32BE(index) & 0x7fffffff) % 1000000).toString().padStart(6, '0')
}
async function login(page: Page, email: string, value: string) {
  await page.goto('/login?next=%2Faccount')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByLabel('Password', { exact: true }).fill(value)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
}
async function english(page: Page) {
  await page.context().addCookies([{ name: 'lang', value: 'en', url: process.env.E2E_SERVER_URL || 'http://localhost:28083' }])
}
async function signOut(page: Page) {
  await page.getByRole('button', { name: 'Sign out', exact: true }).first().click()
  await expect(page).toHaveURL(/\/$/)
  await english(page)
}
async function register(page: Page, email: string) {
  await english(page)
  await page.goto('/signup')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByRole('button', { name: 'Send link' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Check your mail' })).toBeVisible()
  await page.goto(await mailLink(email, 'registration'))
  await expect(page).toHaveURL(/\/link$/)
  await page.getByLabel('New password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Save password' }).click()
  await expect(page).toHaveURL(/\/account$/)
}

test('registers through mail, signs in with a password and resets through mail', async ({ page }) => {
  const email = freshEmail()
  await register(page, email)
  await signOut(page)
  await login(page, email, password)
  await expect(page).toHaveURL(/\/account$/)
  await signOut(page)
  await page.goto('/reset')
  await page.getByLabel('Email', { exact: true }).fill(email)
  await page.getByRole('button', { name: 'Send link' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Check your mail' })).toBeVisible()
  await page.goto(await mailLink(email, 'reset'))
  await page.getByLabel('New password', { exact: true }).fill(replacement)
  await page.getByRole('button', { name: 'Save password' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Password saved' })).toBeVisible()
  await login(page, email, password)
  await expect(page.getByRole('alert')).toContainText('Sign-in failed')
  await login(page, email, replacement)
  await expect(page).toHaveURL(/\/account$/)
})
test('enrolls TOTP and signs in with TOTP and a recovery code', async ({ page }) => {
  const email = freshEmail()
  await register(page, email)
  await page.getByRole('button', { name: 'Set up two-step verification', exact: true }).click()
  const secret = (await page.locator('.auth-secret').textContent())!.trim()
  await page.getByLabel('Authenticator code', { exact: true }).fill(totp(secret, -1))
  await page.getByRole('button', { name: 'Confirm authenticator', exact: true }).click()
  const recovery = (await page.locator('.recovery-codes').textContent())!.trim().split('\n')[0]!
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  await signOut(page)
  await login(page, email, password)
  await expect(page).toHaveURL(/\/login\/factor$/)
  await page.getByLabel('Authenticator code', { exact: true }).fill(totp(secret))
  await page.getByRole('button', { name: 'Verify', exact: true }).click()
  await expect(page).toHaveURL(/\/account$/)
  await signOut(page)
  await login(page, email, password)
  await page.getByRole('button', { name: 'Use a recovery code' }).click()
  await page.getByLabel('Recovery code', { exact: true }).fill(recovery)
  await page.getByRole('button', { name: 'Verify', exact: true }).click()
  await expect(page).toHaveURL(/\/account$/)
})
