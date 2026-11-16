import type { Language } from '../../i18n'

export function usePublicSeo(values: () => { path: string, title: string, description: string, language: Language }) {
  const origin = useRuntimeConfig().public.siteOrigin.replace(/\/$/, '')
  useHead(() => {
    const { path, title, description, language } = values()
    if (!['/', '/vn/topics', '/connect', '/sources', '/privacy', '/terms'].includes(path) && !/^\/vn\/topics\/[^/]+$/.test(path)) return {}
    const canonical = `${origin}${path}`
    return {
      link: [{ rel: 'canonical', href: canonical }],
      meta: [
        { property: 'og:type', content: 'website' },
        { property: 'og:site_name', content: 'Tra Chuẩn' },
        { property: 'og:title', content: title },
        { property: 'og:description', content: description },
        { property: 'og:url', content: canonical },
        { property: 'og:locale', content: language === 'en' ? 'en_US' : 'vi_VN' },
        { property: 'og:image', content: `${origin}/og-image.png` },
        { property: 'og:image:width', content: '1200' },
        { property: 'og:image:height', content: '630' },
        { property: 'og:image:alt', content: 'Tra Chuẩn' },
        { name: 'twitter:card', content: 'summary_large_image' },
      ],
    }
  })
}
