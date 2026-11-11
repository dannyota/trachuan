<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { ApiError } from '../../api/client'
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

async function request(input: SearchInput, retryExpired = false) {
  try {
    const response = await api.client.POST('/api/v1/search', { body: input, params: { header: { 'X-CSRF-Token': api.session.csrfToken || '' } } })
    if (response.data) {
      result.value = response.data
      return
    }
    const code = response.error?.error?.code || 'generic'
    if (code === 'release_expired' && retryExpired) {
      const { release: _release, ...fresh } = input
      await request(fresh)
      return
    }
    error.value = code
    if (response.response.status === 429) retryAfter.value = response.response.headers.get('Retry-After') || ''
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.code : 'generic'
  }
}
async function search(more = false) {
  if (loading.value) return
  if (!more) {
    if (!query.value.trim()) return
    submitted.value = { country: 'vn', query: query.value, ...(asOf.value ? { as_of: asOf.value } : {}), in_force_only: inForce.value, detail: 'full', limit: 20 }
    result.value = null
    expanded.value = false
  }
  if (!submitted.value) return
  const input: SearchInput = more ? { ...submitted.value, limit: 50, ...(result.value?.release ? { release: result.value.release } : {}) } : submitted.value
  loading.value = true
  error.value = ''
  retryAfter.value = ''
  await request(input, more && !!input.release)
  if (more && !error.value) expanded.value = true
  loading.value = false
}
</script>

<template>
  <div>
    <h1>{{ messages.searchTitle }}</h1>
    <form class="search-form" @submit.prevent="search()">
      <label for="search-query">{{ messages.searchQuery }}</label>
      <input id="search-query" v-model="query" autocomplete="off" type="search">
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
