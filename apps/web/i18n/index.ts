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
