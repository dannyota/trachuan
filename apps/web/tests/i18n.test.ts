import { describe, expect, it } from 'vitest'
import { languageFromCookie, messagesFor } from '../i18n'

describe('language selection', () => {
  it('uses Vietnamese for missing or invalid cookies', () => {
    expect(languageFromCookie('')).toBe('vi')
    expect(languageFromCookie('lang=fr')).toBe('vi')
    expect(messagesFor('vi').signIn).toBeTruthy()
  })

  it('selects English only for the English cookie', () => {
    expect(languageFromCookie('other=x; lang=en')).toBe('en')
    expect(messagesFor('en').signIn).toBe('Sign in')
  })
})

it('describes both supported sign-in methods in the privacy copy', () => {
  expect(messagesFor('en').legalIdentity).toContain('password')
  expect(messagesFor('vi').legalIdentity).toContain('mật khẩu')
})
