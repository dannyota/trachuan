<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
import { publicHead } from '../../i18n/public-head'
import { useSessionStore } from '../stores/session'

const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
const messages = computed(() => messagesFor(language.value))
const route = useRoute()
const session = useSessionStore()
const api = useApi()
const siteOrigin = useRuntimeConfig().public.siteOrigin

onMounted(async () => {
  try {
    if (session.status === 'unknown') await api.loadSession()
  } catch {
    session.status = 'signed-out'
  }
})
useHead(() => {
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
  const page = publicHead(language.value, route.path)
  const title = titles[route.path] || page?.title || (route.path.startsWith('/vn/documents/') ? messages.value.reportDocument : '')
  return { htmlAttrs: { lang: language.value }, title: title ? `${title} · Tra Chuẩn` : 'Tra Chuẩn', link: [{ rel: 'icon', type: 'image/svg+xml', href: '/icon.svg' }, { rel: 'icon', href: '/favicon.ico', sizes: 'any' }, { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' }, { rel: 'manifest', href: '/site.webmanifest' }], meta: [...(page ? [{ name: 'description', content: page.description }] : []), { name: 'theme-color', content: '#FFFFFF', media: '(prefers-color-scheme: light)' }, { name: 'theme-color', content: '#111418', media: '(prefers-color-scheme: dark)' }, { property: 'og:title', content: 'Tra Chuẩn' }, { property: 'og:description', content: 'Tra cứu đúng điều, đúng khoản, kèm trạng thái hiệu lực.' }, { property: 'og:image', content: new URL('/og-image.png', siteOrigin).href }, { name: 'twitter:card', content: 'summary_large_image' }] }
})
</script>

<template>
  <div class="site-layout">
    <SiteHeader />
    <main class="site-main"><slot /></main>
    <SiteFooter />
  </div>
</template>
