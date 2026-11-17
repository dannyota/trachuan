import { notFoundDocument } from '../../server/utils/not-found'
import type { Language } from '../../i18n'

export function useFrameworkPage() {
  const language = useState<Language>('language', () => 'vi')
  const api = useApi({ localUnavailable: true })
  const context = import.meta.server ? useNuxtApp().ssrContext : undefined
  function notFound() {
    if (import.meta.server) {
      if (context) context._renderResponse = { statusCode: 404, headers: { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-store', 'x-robots-tag': 'noindex' }, body: notFoundDocument(language.value) }
    } else window.location.reload()
  }
  return { api, language, notFound }
}
