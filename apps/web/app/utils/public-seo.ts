import type { Language } from '../../i18n'

export function isIndexable(path: string): boolean {
  return ['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms'].includes(path) || /^\/vn\/topics\/[^/]+$/.test(path)
}

export function sitePreview(origin: string, language: Language) {
  return [
    { property: 'og:site_name', content: 'Tra Chuẩn' },
    { property: 'og:title', content: 'Tra Chuẩn' },
    { property: 'og:locale', content: language === 'en' ? 'en_US' : 'vi_VN' },
    { property: 'og:image', content: `${origin.replace(/\/$/, '')}/og-image.png` },
    { property: 'og:image:width', content: '1200' },
    { property: 'og:image:height', content: '630' },
    { property: 'og:image:alt', content: 'Tra Chuẩn' },
  ]
}
