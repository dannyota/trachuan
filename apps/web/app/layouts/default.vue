<script setup lang="ts">
import { messagesFor, shellMessages, type Language } from '../../i18n'
import { publicHead } from '../../i18n/public-head'
import { useSessionStore } from '../stores/session'
import { isIndexable } from '../utils/public-seo'

const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
const messages = computed(() => messagesFor(language.value))
const unavailable = useState<boolean>('service-unavailable', () => false)
const route = useRoute()
const topicHead = useState<{ title: string, description: string } | null>('topic-head', () => null)
const session = useSessionStore()
const api = useApi()

onMounted(async () => {
  try {
    if (session.status === 'unknown') await api.loadSession()
  } catch {
    session.status = 'signed-out'
  }
})
const head = computed(() => {
  const titles: Record<string, string> = {
    '/': messages.value.home,
    '/connect': messages.value.connect,
    '/sources': messages.value.sources,
    '/privacy': messages.value.privacy,
    '/terms': messages.value.terms,
    '/login': messages.value.signIn,
    '/account': messages.value.account,
    '/vn/search': messages.value.searchTitle,
  }
  const page = route.path.startsWith('/vn/topics/') && topicHead.value ? topicHead.value : publicHead(language.value, route.path)
  const title = unavailable.value ? shellMessages(language.value).unavailableTitle : titles[route.path] || page?.title || (route.path.startsWith('/vn/documents/') ? messages.value.reportDocument : '')
  return { htmlAttrs: { lang: language.value }, title: !unavailable.value && page && isIndexable(route.path) ? (page.title ?? 'Tra Chuẩn') : title ? `${title} · Tra Chuẩn` : 'Tra Chuẩn', link: [{ rel: 'icon' as const, type: 'image/svg+xml', href: '/icon.svg' }, { rel: 'icon' as const, href: '/favicon.ico', sizes: 'any' }, { rel: 'apple-touch-icon' as const, href: '/apple-touch-icon.png' }, { rel: 'manifest' as const, href: '/site.webmanifest' }], meta: [...(page && isIndexable(route.path) ? [{ name: 'description', content: page.description }] : []), { name: 'theme-color', content: '#FFFFFF', media: '(prefers-color-scheme: light)' }, { name: 'theme-color', content: '#0C1222', media: '(prefers-color-scheme: dark)' }] }
})
useHead(head)
usePublicSeo(() => ({ path: route.path, title: head.value.title, description: (route.path.startsWith('/vn/topics/') && topicHead.value ? topicHead.value : publicHead(language.value, route.path))?.description ?? '', language: language.value }))
</script>

<template>
  <div class="site-layout">
    <a class="skip-link" href="#main">{{ shellMessages(language).skip }}</a>
    <SiteHeader />
    <main id="main" tabindex="-1" class="site-main"><slot /></main>
    <SiteFooter />
  </div>
</template>
