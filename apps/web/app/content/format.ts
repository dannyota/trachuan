import type { Language } from '../../i18n'

export function formatNumber(n: number, lang: Language): string {
  return new Intl.NumberFormat(lang === 'vi' ? 'vi-VN' : 'en-US').format(n)
}

function civilDate(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) return null
  const year = Number(match[1]), month = Number(match[2]), day = Number(match[3])
  const date = new Date(0)
  date.setUTCFullYear(year, month - 1, day)
  date.setUTCHours(0, 0, 0, 0)
  return year > 0 && date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day ? date : null
}

function dateParts(date: Date, lang: Language, timeZone: string): string {
  return new Intl.DateTimeFormat(lang === 'vi' ? 'vi-VN' : 'en-GB', { timeZone, day: '2-digit', month: lang === 'vi' ? '2-digit' : 'short', year: 'numeric' }).format(date)
}

export function formatDate(isoDate: string, lang: Language): string {
  const date = civilDate(isoDate)
  return date ? dateParts(date, lang, 'UTC') : isoDate
}

export function formatDateTime(iso: string, lang: Language): string {
  if (!/^\d{4}-\d{2}-\d{2}T.*(?:Z|[+-]\d{2}:\d{2})$/i.test(iso)) return iso
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  const timeZone = 'Asia/Ho_Chi_Minh'
  const time = new Intl.DateTimeFormat('en-GB', { timeZone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(date)
  return `${dateParts(date, lang, timeZone)}${lang === 'vi' ? ' ' : ', '}${time}`
}

export function formatRelease(rel: string, lang: Language): string {
  const match = /^(\d{4})(\d{2})(\d{2})(?:_(\d+))?$/.exec(rel)
  if (!match) return rel
  const civil = `${match[1]}-${match[2]}-${match[3]}`
  if (!civilDate(civil)) return rel
  const date = formatDate(civil, lang)
  return match[4] ? `${date} (${lang === 'vi' ? 'bản' : 'build'} ${match[4]})` : date
}
