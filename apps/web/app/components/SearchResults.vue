<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { formatNumber, gapCategory } from '../content'
import { resultHits, validityInk } from '../utils/search-results'
import { messagesFor, statusMessage, type Language } from '../../i18n'
import { searchMessages } from '../../i18n/search'
import ResultCard from './ResultCard.vue'

const props = defineProps<{ result: components['schemas']['SearchResult'], query?: string, filters?: string[] }>()
const emit = defineEmits<{ clearFilters: [] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => searchMessages(language.value))
const oldMessages = computed(() => messagesFor(language.value))
const hits = computed(() => resultHits(props.result))
const inside = computed(() => (props.result.hits || []).filter(hit => hit.in_filter !== false))
const outside = computed(() => (props.result.hits || []).filter(hit => hit.in_filter === false))
const statuses = ['in_force', 'partial', 'not_yet', 'expired', 'suspended', 'unknown'] as const
const counts = computed(() => statuses.map(status => ({ status, count: hits.value.filter(hit => {
  const actual = props.result.documents?.[hit.doc || '']?.status.class
  return status === 'unknown' ? !actual || actual === 'unknown' || !statuses.includes(actual as typeof statuses[number]) : actual === status
}).length })).filter(item => item.count))
</script>

<template>
  <section class="search-results" aria-live="polite">
    <div v-if="hits.length" class="search-summary">
      <h1 tabindex="-1">{{ formatNumber(hits.length, language) }} {{ messages.results }}</h1>
      <div class="search-legend"><span v-for="item in counts" :key="item.status" :class="`ink-${validityInk(item.status)}`">{{ statusMessage(language, item.status) }} {{ formatNumber(item.count, language) }}</span></div>
    </div>
    <div v-if="!hits.length" class="notice search-state" role="status"><h1 tabindex="-1">{{ result.abstain ? messages.noEvidence : oldMessages.searchEmpty }}</h1><p>{{ messages.noEvidenceDetail }}</p><p v-if="filters?.length">{{ messages.filteredBy.replace('{filters}', filters.join(', ')) }}</p><button v-if="filters?.length" type="button" class="btn btn-secondary" @click="emit('clearFilters')">{{ messages.clearFilters }}</button></div>
    <div v-else-if="result.abstain && result.reason === 'low_confidence'" class="notice notice--warn search-state" role="status"><strong>{{ messages.weak }}</strong><p>{{ messages.weakDetail }}</p></div>
    <p v-if="result.gaps?.length" class="search-gaps">{{ result.gaps.map(gap => gapCategory(gap.kind, language).name).join('; ') }}</p>
    <ResultCard v-for="(hit, index) in inside" :key="`inside-${hit.handle}-${index}`" :hit="hit" :documents="result.documents" :nodes="result.nodes" :release="result.release" :query="query" :rank="index + 1" />
    <template v-if="outside.length">
      <p class="search-divider">{{ messages.outside }}: {{ formatNumber(result.outside_filter_count ?? outside.length, language) }}</p>
      <ResultCard v-for="(hit, index) in outside" :key="`outside-${hit.handle}-${index}`" :hit="hit" :documents="result.documents" :nodes="result.nodes" :release="result.release" :query="query" :rank="inside.length + index + 1" />
    </template>
    <template v-if="result.non_current?.length">
      <p class="search-divider">{{ messages.nonCurrent }}: {{ formatNumber(result.non_current.length, language) }} {{ messages.results }}</p>
      <details class="search-noncurrent"><summary>{{ messages.showNonCurrent }}</summary><ResultCard v-for="(hit, index) in result.non_current" :key="`past-${hit.handle}-${index}`" :hit="hit" :documents="result.documents" :nodes="result.nodes" :release="result.release" :query="query" :rank="inside.length + outside.length + index + 1" /></details>
    </template>
  </section>
</template>
