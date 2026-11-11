import { expect, it } from 'vitest'
import { loginError } from '../i18n'

it('uses only fixed login messages for known and unknown error codes', () => {
  for (const language of ['vi', 'en'] as const) {
    const messages = ['login_failed', 'login_expired', 'account_blocked'].map(code => loginError(language, code))
    expect(new Set(messages).size).toBe(3)
    expect(loginError(language, 'attacker text')).not.toContain('attacker text')
    expect(loginError(language, 'attacker text')).toBe(loginError(language, 'other'))
  }
})

it('localizes a known document status and hides unknown status codes', async () => {
  const { statusMessage } = await import('../i18n')
  expect(statusMessage('en', 'in_force')).toBe('In force')
  expect(statusMessage('vi', 'in_force')).toBe('Còn hiệu lực')
  expect(statusMessage('en', 'unrecognized')).toBe('Unknown')
})
