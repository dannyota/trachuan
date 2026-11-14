<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { ApiError, type Action } from '../../api/client'
import { isNodeCode } from '../../utils/node-code'
import { appendSearchPage } from '../../utils/search-results'
import { errorMessage, messagesFor, shellMessages, type Language } from '../../../i18n'
import { homeSuggestions } from '../../../i18n/home'
import { searchMessages } from '../../../i18n/search'
import { formatDate } from '../../content'
import { useSearchDraftStore } from '../../stores/search-draft'
import SearchFilters from '../../components/SearchFilters.vue'
import SearchResults from '../../components/SearchResults.vue'

definePageMeta({ signedIn: true, layout: false })
type SearchInput = components['schemas']['SearchInput']
type SearchResult = components['schemas']['SearchResult']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => searchMessages(language.value))
const draft = useSearchDraftStore()
const dialog = ref<HTMLDialogElement | null>(null)
const filterButton = ref<HTMLButtonElement | null>(null)
const dialogOpen = ref(false)
const api = useApi({ userAction: true, localUnavailable: true })
const filterApi = useApi({ localUnavailable: true })
const route = useRoute()
const router = useRouter()
const searchMain = ref<HTMLElement | null>(null)
const initialNode = typeof route.query.node === 'string' && isNodeCode(route.query.node) ? route.query.node : ''
const nodes = ref<string[]>(initialNode ? [initialNode] : [])
const nodeLabels = ref<Record<string, string>>({})
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
const rateLimited = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
async function loadFacets() {
  try {
    const response = await filterApi.client.GET('/api/v1/status')
    if (!response.data) throw new Error('unavailable')
    facets.value = response.data
    facetsFailed.value = false
  } catch { facetsFailed.value = true }
}
onMounted(() => {
  if (Object.keys(route.query).some(key => key !== 'node')) {
    router.replace({ query: route.query.node === undefined ? {} : { node: route.query.node }, hash: route.hash })
  }
  loadFacets()
  if (draft.draft) {
    query.value = draft.draft.query
    inForce.value = draft.draft.inForceOnly
    draft.draft = null
    search()
  }
})
const activeCount = computed(() => Number(inForce.value) + Number(!!asOf.value) + nodes.value.length + docTypes.value.length + issuers.value.length)
const activeFilters = computed(() => [
  ...(inForce.value ? [messages.value.searchInForce] : []),
  ...(asOf.value ? [formatDate(asOf.value, language.value)] : []),
  ...nodes.value.map(node => nodeLabels.value[node] || node), ...docTypes.value, ...issuers.value,
])
async function focusOutcome() {
  await nextTick()
  searchMain.value?.querySelector<HTMLElement>(error.value ? '[role="alert"] h1' : '.search-results h1')?.focus()
}
async function clearFilters() {
  const submittedQuery = submitted.value?.query
  if (!submittedQuery || loading.value) return
  inForce.value = false; asOf.value = ''; nodes.value = []; docTypes.value = []; issuers.value = []
  await search(false, submittedQuery)
  await focusOutcome()
}
async function retrySearch() {
  if (!failedRequest || !action || !controller || loading.value) return
  await runRequest(failedRequest.input, failedRequest.more)
  await focusOutcome()
}
async function openFilters() {
  dialogOpen.value = true
  await nextTick()
  dialog.value?.showModal()
}
function closeFilters() { dialog.value?.close() }
function dialogClosed() { dialogOpen.value = false; filterButton.value?.focus() }
function fillExample(value: string) { query.value = value }
function onQueryKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.ctrlKey || event.altKey || event.metaKey || event.isComposing) return
  event.preventDefault()
  search()
}

let action: Action | null = null
let controller: AbortController | null = null
let failedRequest: { input: SearchInput, more: boolean } | null = null
onBeforeUnmount(() => controller?.abort())

async function request(input: SearchInput, captured: Action, signal: AbortSignal, more: boolean) {
  try {
    const response = await api.forAction(captured).POST('/api/v1/search', { body: input, signal, params: { header: { 'X-CSRF-Token': captured.csrfToken || '' } } })
    if (signal.aborted) return
    if (api.session.account?.id !== captured.accountId) { error.value = 'account_changed'; result.value = null; return }
    if (response.data) {
      if (more && result.value) {
        result.value = appendSearchPage(result.value, response.data)
      } else result.value = response.data
      return
    }
    if (!more) result.value = null
    const code = response.error?.error?.code || 'generic'
    if (more && code === 'release_expired' && result.value) result.value.next_cursor = undefined
    error.value = code
    failedRequest = { input, more }
    if (response.response.status === 429) { rateLimited.value = true; retryAfter.value = response.response.headers.get('Retry-After') || '' }
  } catch (cause) {
    if (signal.aborted) return
    if (!more) result.value = null
    error.value = cause instanceof ApiError ? cause.code : 'generic'
    failedRequest = { input, more }
  }
}
async function search(more = false, submittedQuery = query.value) {
  if (loading.value) return
  if (!more) {
    if (!submittedQuery.trim()) return
    controller?.abort()
    controller = new AbortController()
    action = api.captureAction()
    submitted.value = { country: 'vn', query: submittedQuery, ...(asOf.value ? { as_of: asOf.value } : {}), in_force_only: inForce.value, detail: 'full', limit: 20, ...(nodes.value.length ? { nodes: [...nodes.value] } : {}), ...(docTypes.value.length ? { doc_types: [...docTypes.value] } : {}), ...(issuers.value.length ? { issuers: [...issuers.value] } : {}) }
  }
  if (!submitted.value || !action || !controller) return
  if (more && !result.value?.next_cursor) return
  const input: SearchInput = more ? { ...submitted.value, cursor: result.value!.next_cursor, release: result.value!.release } : submitted.value
  await runRequest(input, more)
}
async function runRequest(input: SearchInput, more: boolean) {
  if (!action || !controller) return
  failedRequest = null
  loading.value = true
  loadingMore.value = more
  error.value = ''
  retryAfter.value = ''
  rateLimited.value = false
  const signal = controller.signal
  await request(input, action, signal, more)
  if (signal.aborted) return
  loading.value = false
}
</script>

<template>
  <NuxtLayout name="search">
    <template #header>
      <div class="search-header">
        <form class="search-box" @submit.prevent="search()">
          <label for="search-query" class="visually-hidden">{{ messages.searchQuery }}</label>
          <textarea id="search-query" v-model="query" class="input" rows="1" autocomplete="off" :placeholder="messages.searchQuery" @keydown="onQueryKeydown" />
          <button class="btn btn-primary" type="submit" :disabled="loading">{{ messages.searchSubmit }}</button>
        </form>
        <div class="search-mobile-chips">
          <button ref="filterButton" class="chip" type="button" @click="openFilters">{{ labels.filters }} · {{ activeCount }}</button>
          <button v-if="inForce" type="button" class="chip" @click="inForce = false">{{ messages.searchInForce }} ×</button>
          <button v-if="asOf" type="button" class="chip" @click="asOf = ''">{{ formatDate(asOf, language) }} ×</button>
          <button v-for="type in docTypes" :key="type" type="button" class="chip" @click="docTypes = docTypes.filter(item => item !== type)">{{ type }} ×</button>
          <button v-for="issuer in issuers" :key="issuer" type="button" class="chip" @click="issuers = issuers.filter(item => item !== issuer)">{{ issuer }} ×</button>
          <button v-for="node in nodes" :key="node" type="button" class="chip" @click="nodes = nodes.filter(item => item !== node)">{{ nodeLabels[node] || messages.searchNodes }} ×</button>
        </div>
      </div>
    </template>
    <div class="search-page">
      <aside class="search-rail" :aria-label="labels.filters"><SearchFilters v-model:in-force="inForce" v-model:as-of="asOf" v-model:nodes="nodes" v-model:doc-types="docTypes" v-model:issuers="issuers" :facets="facets" :failed="facetsFailed" @retry="loadFacets" @labels="nodeLabels = { ...nodeLabels, ...$event }" /></aside>
      <dialog v-if="dialogOpen" ref="dialog" class="search-dialog" :aria-label="labels.filters" @close="dialogClosed">
        <button type="button" class="btn btn-secondary search-dialog-close" @click="closeFilters">{{ labels.close }}</button>
        <SearchFilters v-model:in-force="inForce" v-model:as-of="asOf" v-model:nodes="nodes" v-model:doc-types="docTypes" v-model:issuers="issuers" :facets="facets" :failed="facetsFailed" @retry="loadFacets" @labels="nodeLabels = { ...nodeLabels, ...$event }" />
      </dialog>
      <div ref="searchMain" class="search-main" aria-live="polite">
        <div v-if="loading" class="search-loading" role="status"><span class="search-spinner" aria-hidden="true" />{{ labels.loading }}</div>
        <div v-if="error" class="notice search-state" role="alert"><h1 tabindex="-1">{{ rateLimited ? labels.rate : error === 'unavailable' ? labels.unavailable : errorMessage(language, error) }}</h1><button v-if="error === 'unavailable'" type="button" class="btn btn-secondary" @click="retrySearch">{{ shellMessages(language).retry }}</button><span v-if="retryAfter">{{ messages.searchRetry }} {{ retryAfter }} {{ messages.searchSeconds }}</span></div>
        <div v-if="result" :class="{ 'search-results-pending': loading && !loadingMore }"><SearchResults :result="result" :query="submitted?.query" :filters="activeFilters" @clear-filters="clearFilters" /></div>
        <div v-else-if="!loading && !error" class="notice search-state search-idle"><h1>{{ messages.searchTitle }}</h1><p>{{ labels.idle }}</p><p>{{ labels.examples }}</p><button v-for="suggestion in homeSuggestions" :key="suggestion" type="button" class="chip" @click="fillExample(suggestion)">{{ suggestion }}</button></div>
        <button v-if="result?.next_cursor" type="button" class="btn btn-secondary search-more" :disabled="loading" @click="search(true)">{{ labels.more }}</button>
      </div>
    </div>
  </NuxtLayout>
</template>
