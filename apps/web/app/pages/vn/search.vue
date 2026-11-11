<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { ApiError, type Action } from '../../api/client'
import { errorMessage, messagesFor, type Language } from '../../../i18n'

definePageMeta({ signedIn: true })
type SearchInput = components['schemas']['SearchInput']
type SearchResult = components['schemas']['SearchResult']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi({ userAction: true })
const query = ref('')
const asOf = ref('')
const inForce = ref(true)
const result = ref<SearchResult | null>(null)
const submitted = ref<SearchInput | null>(null)
const error = ref('')
const retryAfter = ref('')
const loading = ref(false)
const expanded = ref(false)
let action: Action | null = null
let controller: AbortController | null = null
onBeforeUnmount(() => controller?.abort())

async function request(input: SearchInput, captured: Action, signal: AbortSignal, retryExpired = false) {
  try {
    const response = await api.forAction(captured).POST('/api/v1/search', { body: input, signal, params: { header: { 'X-CSRF-Token': captured.csrfToken || '' } } })
    if (signal.aborted) return
    if (api.session.account?.id !== captured.accountId) { error.value = 'account_changed'; result.value = null; return }
    if (response.data) {
      result.value = response.data
      return
    }
    const code = response.error?.error?.code || 'generic'
    if (code === 'release_expired' && retryExpired) {
      const { release: _release, ...fresh } = input
      await request(fresh, captured, signal)
      return
    }
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
    submitted.value = { country: 'vn', query: query.value, ...(asOf.value ? { as_of: asOf.value } : {}), in_force_only: inForce.value, detail: 'full', limit: 20 }
    result.value = null
    expanded.value = false
  }
  if (!submitted.value || !action || !controller) return
  const input: SearchInput = more ? { ...submitted.value, limit: 50, ...(result.value?.release ? { release: result.value.release } : {}) } : submitted.value
  loading.value = true
  error.value = ''
  retryAfter.value = ''
  const signal = controller.signal
  await request(input, action, signal, more && !!input.release)
  if (signal.aborted) return
  if (more && !error.value) expanded.value = true
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
      <button type="submit" :disabled="loading">{{ messages.searchSubmit }}</button>
    </form>
    <p v-if="error" role="alert">{{ errorMessage(language, error) }} <span v-if="retryAfter">{{ messages.searchRetry }} {{ retryAfter }} {{ messages.searchSeconds }}</span></p>
    <SearchResults v-if="result" :result="result" />
    <button v-if="result && !result.abstain && !expanded && (result.hits?.length || result.non_current?.length)" type="button" :disabled="loading" @click="search(true)">{{ messages.searchMore }}</button>
  </div>
</template>
