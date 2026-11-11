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
