<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { frameworkMessages } from '../../../i18n/frameworks'
import { errorMessage } from '../../../i18n'
import { ApiError, type Action } from '../../api/client'
import { controlPath } from '../../utils/framework-handles'
import FrameworkAccess from '../../components/FrameworkAccess.vue'

definePageMeta({ signedIn: true })
type Result = components['schemas']['FrameworkSearchResult']
const { api, language, notFound } = useFrameworkPage()
const labels = computed(() => frameworkMessages(language.value))
const query = ref('')
const filter = ref('')
const submitted = ref('')
const selected = ref('')
const result = ref<Result>()
const error = ref('')
const notice = ref('')
const loading = ref(false)
let action: Action | undefined
const { data: status, clear } = await useAsyncData('framework-search-status', async () => {
  const response = await api.client.GET('/api/v1/frameworks')
  if (response.response.status === 404) notFound()
  if (!response.data) error.value = response.error?.error.code || 'unavailable'
  return response.data || null
})
onBeforeUnmount(clear)
const codes = computed(() => [...new Set(Object.keys(status.value?.versions || {}).map(handle => handle.split(':')[1]).filter((code): code is string => !!code))])
async function search(more = false, restart = false) {
  if (loading.value) return
  if (!more && !restart) { if (!query.value.trim()) return; submitted.value = query.value; selected.value = filter.value; notice.value = ''; action = api.captureAction() }
  if (!action) return
  loading.value = true
  error.value = ''
  let response
  try {
    response = await api.forAction(action).POST('/api/v1/frameworks/search', { params: { header: { 'X-CSRF-Token': action.csrfToken || '' } }, body: { corpus: 'frameworks', query: submitted.value, ...(selected.value ? { frameworks: [selected.value] } : {}), ...(more && result.value ? { cursor: result.value.next_cursor, release: result.value.release } : {}) } })
  } catch (cause) {
    loading.value = false
    error.value = cause instanceof ApiError ? cause.code : 'unavailable'
    return
  }
  loading.value = false
  if (response.response.status === 404) { notFound(); return }
  if (!response.data) {
    const code = response.error?.error.code || 'unavailable'
    if (more && code === 'cursor_expired') { notice.value = errorMessage(language.value, code); result.value = undefined; await search(false, true); return }
    error.value = code
    return
  }
  result.value = more && result.value ? { ...response.data, hits: [...(result.value.hits || []), ...(response.data.hits || [])] } : response.data
}
</script>
<template>
  <main class="container page-content fw-page">
    <h1>{{ labels.frameworks }}</h1>
    <form class="fw-form" @submit.prevent="search()"><CorpusControl current="frameworks" /><label>{{ labels.search }} <input v-model="query" class="input" autocomplete="off"></label><label>{{ labels.filter }} <select v-model="filter" class="input"><option value="">{{ labels.all }}</option><option v-for="code in codes" :key="code" :value="code">{{ code }}</option></select></label><button class="btn btn-primary" type="submit" :disabled="loading">{{ labels.search }}</button></form>
    <p v-if="status?.access_unavailable || result?.access_unavailable" class="notice">{{ labels.accessUnavailable }}</p>
    <p v-if="notice" role="status">{{ notice }}</p><p v-if="error" role="alert">{{ errorMessage(language, error) }}</p>
    <p v-if="result?.abstain">{{ result.message }}</p>
    <article v-for="hit in result?.hits" :key="hit.handle" class="fw-row">
      <h2 class="fw-hit-identifier"><NuxtLink :to="controlPath(hit.handle, hit.citation)" class="fw-mono">{{ hit.citation }}</NuxtLink></h2>
      <p v-if="hit.title" class="fw-title">{{ hit.title }}</p>
      <p class="fw-meta">{{ [hit.kind, result?.frameworks?.[hit.handle.split('#')[0]!]?.name, hit.lookup ? labels.lookup : hit.score, `${hit.mappings} ${labels.mappings}`].filter(value => value !== undefined && value !== '').join(' · ') }}<template v-if="hit.text_withheld && result?.frameworks?.[hit.handle.split('#')[0]!]"><span> · </span><FrameworkAccess :access="result.frameworks[hit.handle.split('#')[0]!]!.access" /></template></p>
      <p v-if="hit.snippet" class="fw-source">{{ hit.snippet }}</p>
    </article>
    <button v-if="result?.next_cursor" class="btn btn-secondary" type="button" :disabled="loading" @click="search(true)">{{ labels.more }}</button>
  </main>
</template>
<style scoped>
.fw-hit-identifier { margin: 0; font-size: 16px; }
.fw-mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.fw-source { font-family: var(--font-serif); white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
