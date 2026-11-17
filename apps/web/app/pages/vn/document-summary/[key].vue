<script setup lang="ts">
import { setResponseHeader } from 'h3'
import { createSummaryApi } from '../../../api/summary'
import { summaryGraph } from '../../../utils/structured-data'
import { summarySeo } from '../../../utils/summary-seo'
import { isDocumentKey } from '../../../utils/document-key'
import { publicStatus } from '../../../../server/utils/public-status'
import { notFoundDocument } from '../../../../server/utils/not-found'
import { messagesFor, shellMessages, type Language } from '../../../../i18n'

definePageMeta({ path: '/vn/documents/:key/summary', layout: 'summary', key: route => route.path })
const route = useRoute()
const key = String(route.params.key)
const language = useState<Language>('language', () => 'vi')
const event = import.meta.server ? useRequestEvent() : undefined
const context = import.meta.server ? useNuxtApp().ssrContext : undefined
const origin = useRuntimeConfig().public.siteOrigin.replace(/\/$/, '')
const apiOrigin = import.meta.server ? useRuntimeConfig().apiOrigin : window.location.origin
const api = createSummaryApi(apiOrigin)
const { data } = await useAsyncData(`summary-${key}`, async () => {
  try {
    if (!isDocumentKey(key)) return { kind: 'missing' as const }
    const signal = AbortSignal.timeout(3000)
    let status = import.meta.server ? await publicStatus(apiOrigin) : (await api.status(signal)).data
    if (!status) return { kind: 'unavailable' as const }
    if (!status.features?.public_summaries) return { kind: 'missing' as const }
    const requested = Date.now()
    const result = await api.summary(key, signal)
    if (result.response.status === 404) return { kind: 'missing' as const }
    if (!result.data) return { kind: 'unavailable' as const }
    if (import.meta.server && result.data.release !== status.release) {
      status = await publicStatus(apiOrigin, requested)
      if (!status) return { kind: 'unavailable' as const }
      if (!status.features?.public_summaries) return { kind: 'missing' as const }
    }
    if (result.data.release !== status.release) return { kind: 'unavailable' as const }
    return { kind: 'summary' as const, summary: result.data }
  } catch { return { kind: 'unavailable' as const } }
})
const summary = computed(() => data.value?.kind === 'summary' ? data.value.summary : undefined)
if (data.value?.kind === 'missing') {
  if (context) context._renderResponse = { statusCode: 404, headers: { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-cache' }, body: notFoundDocument(language.value) }
  if (import.meta.client) await navigateTo(route.path, { external: true })
} else if (!summary.value && event) {
  setResponseStatus(event, 503)
  setResponseHeader(event, 'Cache-Control', 'no-cache')
}
if (event && !summary.value?.index) setResponseHeader(event, 'X-Robots-Tag', 'noindex')
const head = computed(() => summary.value ? summarySeo(summary.value.document) : { title: shellMessages(language.value).unavailableTitle, description: '' })
useHead(() => ({ title: head.value.title, meta: [...(summary.value ? [{ name: 'description', content: head.value.description }] : []), ...(!summary.value?.index ? [{ name: 'robots', content: 'noindex' }] : [])] }))
usePublicSeo(() => ({ path: route.path, ...head.value, language: language.value, summary: !!summary.value }))
useStructuredData(() => summary.value ? summaryGraph(origin, summary.value) : [])
function retry() { window.location.reload() }
</script>

<template>
  <div>
    <DocumentSummary v-if="summary" :summary="summary" />
    <div v-else class="container page-content"><div class="notice search-state" role="alert"><h1>{{ shellMessages(language).unavailableTitle }}</h1><p>{{ messagesFor(language).errors.unavailable }}</p><button class="btn btn-secondary" type="button" @click="retry">{{ shellMessages(language).retry }}</button></div></div>
  </div>
</template>
