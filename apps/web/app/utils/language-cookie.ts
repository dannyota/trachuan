import type { Language } from '../../i18n'

const lifetime = 31536000
const storeTimeout = 1000
const readLimit = 50

async function storeCookie(language: Language): Promise<boolean> {
  if (typeof cookieStore === 'undefined') return false
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<false>(resolve => { timer = setTimeout(() => resolve(false), storeTimeout) })
  const write = cookieStore.set({ name: 'lang', value: language, path: '/', sameSite: 'lax', expires: Date.now() + lifetime * 1000 }).then(() => true, () => false)
  try {
    return await Promise.race([write, timeout])
  } finally {
    clearTimeout(timer)
  }
}

function holds(pair: string): boolean {
  return document.cookie.split('; ').includes(pair)
}

export async function saveLanguage(language: Language): Promise<void> {
  if (await storeCookie(language)) return
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(location.hostname)
  const secure = location.protocol === 'http:' && loopback ? '' : '; Secure'
  document.cookie = `lang=${language}; Path=/; Max-Age=${lifetime}; SameSite=Lax${secure}`
  for (let i = 0; i < readLimit && !holds(`lang=${language}`); i++);
}
