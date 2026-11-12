<script setup lang="ts">
import { createApi } from '../../../api/client'
import { messagesFor, type Language } from '../../../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const event = import.meta.server ? useRequestEvent() : undefined
const api = createApi({ origin: import.meta.server ? useRuntimeConfig().apiOrigin : window.location.origin, publicRoute: true })
const { data } = await useAsyncData('topics', async () => {
  if (!import.meta.server) return null
  const response = await api.client.GET('/api/v1/public/taxonomy', { params: { query: { depth: 1 } } })
  if (!response.data) { setResponseStatus(event!, 503); return null }
  return response.data
})
</script>
<template><div><h1>{{ messages.topics }}</h1><TopicTree v-if="data" :nodes="data.nodes || []" /><p v-else>{{ messages.errors.unavailable }}</p></div></template>
