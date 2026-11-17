import type { Language } from '../../i18n'
import { isIndexable, sitePreview } from '../utils/public-seo'

export function usePublicSeo(values: () => { path: string, title: string, description: string, language: Language, summary?: boolean }) {
  const origin = useRuntimeConfig().public.siteOrigin.replace(/\/$/, '')
  useHead(() => {
    const { path, title, description, language, summary } = values()
    const indexable = summary || isIndexable(path)
    const canonical = `${origin}${path}`
    const preview = sitePreview(origin, language).map(tag => tag.property === 'og:title' && indexable ? { ...tag, content: title } : tag)
    return {
      link: indexable ? [{ rel: 'canonical', href: canonical }] : [],
      meta: [
        ...preview,
        ...(indexable ? [
          { property: 'og:type', content: 'website' },
          { property: 'og:description', content: description },
          { property: 'og:url', content: canonical },
        ] : []),
        { name: 'twitter:card', content: 'summary_large_image' },
      ],
    }
  })
}
