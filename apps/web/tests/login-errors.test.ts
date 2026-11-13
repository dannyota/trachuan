import { expect, it } from 'vitest'
import { loginError, messagesFor } from '../i18n'

it('uses only fixed login messages for known and unknown error codes', () => {
  for (const language of ['vi', 'en'] as const) {
    const messages = ['login_failed', 'login_expired', 'account_blocked', 'signup_closed'].map(code => loginError(language, code))
    expect(new Set(messages).size).toBe(4)
    expect(loginError(language, 'attacker text')).not.toContain('attacker text')
    expect(loginError(language, 'attacker text')).toBe(loginError(language, 'other'))
    expect(loginError(language, 'attacker text')).toBe(messagesFor(language).loginGeneric)
    expect(loginError(language, 'attacker text')).not.toBe(loginError(language, 'signup_closed'))
  }
  expect(loginError('vi', 'signup_closed')).toBe('Tra Chuẩn chưa mở đăng ký tài khoản mới.')
  expect(loginError('en', 'signup_closed')).toBe('Tra Chuẩn is not open to new accounts yet.')
})

it('localizes a known document status and hides unknown status codes', async () => {
  const { statusMessage } = await import('../i18n')
  expect(statusMessage('en', 'in_force')).toBe('In force')
  expect(statusMessage('vi', 'in_force')).toBe('Còn hiệu lực')
  expect(statusMessage('en', 'unrecognized')).toBe('Unknown')
})
