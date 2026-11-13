<script setup lang="ts">
import { createApi } from '../../../api/client'
import { messagesFor, type Language } from '../../../../i18n'
import { pagesMessages } from '../../../../i18n/pages'
const clientEntry = import.meta.client && !useNuxtApp().isHydrating
onMounted(() => { if (clientEntry) window.location.reload() })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => pagesMessages(language.value))
const event = import.meta.server ? useRequestEvent() : undefined
const api = createApi({ origin: import.meta.server ? useRuntimeConfig().apiOrigin : window.location.origin, publicRoute: true })
const { data } = await useAsyncData('topics', async () => {
  if (!import.meta.server) return null
  const response = await api.client.GET('/api/v1/public/taxonomy', { params: { query: { depth: 1 } } })
  if (!response.data) { setResponseStatus(event!, 503); setResponseHeader(event!, 'Cache-Control', 'no-cache'); return null }
  return response.data
})
</script>
<template><div class="container page-content"><header class="page-intro"><h1>{{ messages.topics }}</h1><p>{{ labels.topicIntro }}</p></header><TopicTree v-if="data" :nodes="data.nodes || []" /><p v-else role="status">{{ messages.errors.unavailable }}</p></div></template>
