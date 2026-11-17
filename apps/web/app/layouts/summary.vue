<script setup lang="ts">
import { shellMessages, type Language } from '../../i18n'

const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
useHead(() => ({ htmlAttrs: { lang: language.value }, link: [{ rel: 'icon', type: 'image/svg+xml', href: '/icon.svg' }], meta: [{ name: 'theme-color', content: '#FFFFFF', media: '(prefers-color-scheme: light)' }, { name: 'theme-color', content: '#0C1222', media: '(prefers-color-scheme: dark)' }] }))
</script>

<template>
  <div class="site-layout">
    <a class="skip-link" href="#main">{{ shellMessages(language).skip }}</a>
    <SummaryHeader />
    <main id="main" tabindex="-1" class="site-main reader-main"><slot /></main>
    <SiteFooter />
  </div>
</template>
