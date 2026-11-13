import { en } from './en'
import { vi } from './vi'

export type Language = 'vi' | 'en'

export function languageFromCookie(cookie: string): Language {
  const value = cookie.split(';').map(part => part.trim())
    .find(part => part.startsWith('lang='))?.slice(5)
  return value === 'en' ? 'en' : 'vi'
}

export function messagesFor(language: Language): typeof en {
  return language === 'en' ? en : vi
}

export function errorMessage(language: Language, code: string): string {
  const errors = messagesFor(language).errors
  return Object.hasOwn(errors, code) ? errors[code as keyof typeof errors] : errors.generic
}

export function loginError(language: Language, code: unknown): string {
  const messages = messagesFor(language)
  switch (code) {
    case 'login_failed': return messages.loginFailed
    case 'login_expired': return messages.loginExpired
    case 'account_blocked': return messages.loginBlocked
    case 'signup_closed': return messages.signupClosed
    default: return messages.loginGeneric
  }
}

export function statusMessage(language: Language, status: string | undefined): string {
  const messages = messagesFor(language)
  switch (status) {
    case 'in_force': return messages.statusInForce
    case 'partial': return messages.statusPartial
    case 'expired': return messages.statusExpired
    case 'not_yet': return messages.statusNotYet
    case 'suspended': return messages.statusSuspended
    default: return messages.statusUnknown
  }
}
