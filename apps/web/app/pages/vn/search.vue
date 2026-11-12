<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { ApiError, type Action } from '../../api/client'
import { isNodeCode } from '../../utils/node-code'
import { errorMessage, messagesFor, type Language } from '../../../i18n'

definePageMeta({ signedIn: true })
type SearchInput = components['schemas']['SearchInput']
type SearchResult = components['schemas']['SearchResult']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi({ userAction: true })
const filterApi = useApi({ localUnavailable: true })
const route = useRoute()
const initialNode = typeof route.query.node === 'string' && isNodeCode(route.query.node) ? route.query.node : ''
const nodes = ref<string[]>(initialNode ? [initialNode] : [])
const docTypes = ref<string[]>([])
const issuers = ref<string[]>([])
const facets = ref<components['schemas']['CorpusStatus'] | null>(null)
const facetsFailed = ref(false)
const query = ref('')
const asOf = ref('')
const inForce = ref(true)
const result = ref<SearchResult | null>(null)
const submitted = ref<SearchInput | null>(null)
const error = ref('')
const retryAfter = ref('')
const loading = ref(false)
async function loadFacets() {
  try {
    const response = await filterApi.client.GET('/api/v1/status')
    if (!response.data) throw new Error('unavailable')
    facets.value = response.data
    facetsFailed.value = false
  } catch { facetsFailed.value = true }
}
onMounted(loadFacets)
let action: Action | null = null
let controller: AbortController | null = null
onBeforeUnmount(() => controller?.abort())

async function request(input: SearchInput, captured: Action, signal: AbortSignal, more: boolean) {
  try {
    const response = await api.forAction(captured).POST('/api/v1/search', { body: input, signal, params: { header: { 'X-CSRF-Token': captured.csrfToken || '' } } })
    if (signal.aborted) return
    if (api.session.account?.id !== captured.accountId) { error.value = 'account_changed'; result.value = null; return }
    if (response.data) {
      if (more && result.value) {
        const previous = result.value
        result.value = { ...previous, ...response.data,
          next_cursor: response.data.next_cursor,
          documents: { ...previous.documents, ...response.data.documents },
          nodes: { ...previous.nodes, ...response.data.nodes },
          hits: [...(previous.hits || []), ...(previous.non_current || []), ...(response.data.hits || []), ...(response.data.non_current || [])], non_current: [],
        }
      } else result.value = response.data
      return
    }
    const code = response.error?.error?.code || 'generic'
    if (more && code === 'release_expired' && result.value) result.value.next_cursor = undefined
    error.value = code
    if (response.response.status === 429) retryAfter.value = response.response.headers.get('Retry-After') || ''
  } catch (cause) {
    if (signal.aborted) return
    error.value = cause instanceof ApiError ? cause.code : 'generic'
  }
}
async function search(more = false) {
  if (loading.value) return
  if (!more) {
    if (!query.value.trim()) return
    controller?.abort()
    controller = new AbortController()
    action = api.captureAction()
    submitted.value = { country: 'vn', query: query.value, ...(asOf.value ? { as_of: asOf.value } : {}), in_force_only: inForce.value, detail: 'full', limit: 20, ...(nodes.value.length ? { nodes: [...nodes.value] } : {}), ...(docTypes.value.length ? { doc_types: [...docTypes.value] } : {}), ...(issuers.value.length ? { issuers: [...issuers.value] } : {}) }
    result.value = null
  }
  if (!submitted.value || !action || !controller) return
  if (more && !result.value?.next_cursor) return
  const input: SearchInput = more ? { ...submitted.value, cursor: result.value!.next_cursor, release: result.value!.release } : submitted.value
  loading.value = true
  error.value = ''
  retryAfter.value = ''
  const signal = controller.signal
  await request(input, action, signal, more)
  if (signal.aborted) return
  loading.value = false
}
</script>

<template>
  <div>
    <h1>{{ messages.searchTitle }}</h1>
    <form class="search-form" @submit.prevent="search()">
      <label for="search-query">{{ messages.searchQuery }}</label>
      <textarea id="search-query" v-model="query" autocomplete="off" />
      <label for="search-date">{{ messages.searchDate }}</label>
      <input id="search-date" v-model="asOf" type="date" autocomplete="off">
      <label><input v-model="inForce" type="checkbox" autocomplete="off"> {{ messages.searchInForce }}</label>
      <SearchNodePicker v-model="nodes" />
      <p v-if="facetsFailed" role="status">{{ messages.filtersUnavailable }} <button type="button" @click="loadFacets">{{ messages.retry }}</button></p>
      <label>{{ messages.searchTypes }} <select v-model="docTypes" name="types" multiple @change="docTypes = docTypes.slice(0, 10)"><option v-for="facet in facets?.doc_types" :key="facet.value" :value="facet.value">{{ facet.value }} ({{ facet.count }})</option></select></label>
      <label>{{ messages.searchIssuers }} <select v-model="issuers" name="issuers" multiple @change="issuers = issuers.slice(0, 10)"><option v-for="facet in facets?.issuers" :key="facet.value" :value="facet.value">{{ facet.value }} ({{ facet.count }})</option></select></label>
      <button type="submit" :disabled="loading">{{ messages.searchSubmit }}</button>
    </form>
    <p v-if="error" role="alert">{{ errorMessage(language, error) }} <span v-if="retryAfter">{{ messages.searchRetry }} {{ retryAfter }} {{ messages.searchSeconds }}</span></p>
    <SearchResults v-if="result" :result="result" />
    <button v-if="result?.next_cursor" type="button" :disabled="loading" @click="search(true)">{{ messages.searchMore }}</button>
  </div>
</template>
