<script setup lang="ts">
import { notFoundDocument } from '../../../../server/utils/not-found'
import { createApi } from '../../../api/client'
import { messagesFor, type Language } from '../../../../i18n'
const clientEntry = import.meta.client && !useNuxtApp().isHydrating
onMounted(() => { if (clientEntry) window.location.reload() })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const route = useRoute()
const code = String(route.params.code)
const context = import.meta.server ? useNuxtApp().ssrContext : undefined
const event = import.meta.server ? useRequestEvent() : undefined
const api = createApi({ origin: import.meta.server ? useRuntimeConfig().apiOrigin : window.location.origin, publicRoute: true })
const { data } = await useAsyncData(`topic-${code}`, async () => {
  if (!import.meta.server) return null
  const response = await api.client.GET('/api/v1/public/taxonomy/{code}', { params: { path: { code } } })
  if (response.response.status === 404) {
    if (context) context._renderResponse = { statusCode: 404, headers: { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-cache' }, body: notFoundDocument(language.value) }
    return null
  }
  if (!response.data) { setResponseStatus(event!, 503); setResponseHeader(event!, 'Cache-Control', 'no-cache'); return null }
  return response.data
})
</script>
<template><TopicNode v-if="data" :page="data" /><p v-else>{{ messages.errors.unavailable }}</p></template>
