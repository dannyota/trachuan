<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, statusMessage, type Language } from '../../i18n'
import { readerLink, resultHits, validityInk } from '../utils/search-results'

const props = defineProps<{ result: components['schemas']['SearchResult'] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const hits = computed(() => resultHits(props.result))
const inside = computed(() => hits.value.filter(hit => hit.in_filter !== false))
const outside = computed(() => hits.value.filter(hit => hit.in_filter === false))
const display = computed(() => [...inside.value, ...outside.value])
const inks = ['in-force', 'partial', 'not-yet', 'expired', 'neutral'] as const
const counts = computed(() => Object.fromEntries(inks.map(ink => [ink, hits.value.filter(hit => validityInk(hit.doc ? props.result.documents?.[hit.doc]?.status.class : undefined) === ink).length])))
const hitGapCounts = computed(() => {
  const counts: Record<string, number> = Object.create(null)
  for (const hit of hits.value) for (const gap of hit.gaps || []) {
    const kind = gap.kind || ''
    counts[kind] = (counts[kind] || 0) + 1
  }
  return counts
})
function gapLabel(kind: string | undefined) {
  switch (kind) {
    case 'validity_conflict': return messages.value.gapValidityConflict
    case 'missing_effective_date': return messages.value.gapMissingEffectiveDate
    case 'consolidated_status_unordered': return messages.value.gapConsolidatedUnordered
    case 'consolidated_status_unresolved': return messages.value.gapConsolidatedUnresolved
    case 'ambiguous_tree_text': return messages.value.gapAmbiguousText
    default: return kind || messages.value.statusUnknown
  }
}
function label(status: string | undefined) {
  return validityInk(status) === 'neutral' && status ? status : statusMessage(language.value, status)
}
</script>

<template>
  <section class="search-results" aria-live="polite">
    <div class="search-summary">
      <span v-for="(status, index) in ['in_force', 'partial', 'not_yet', 'expired', 'unknown']" :key="status" :class="`ink-${inks[index]}`">{{ label(status) }}: {{ counts[inks[index]!] }}</span>
      <div v-if="Object.keys(hitGapCounts).length"><strong>{{ messages.gaps }}: {{ Object.values(hitGapCounts).reduce((total, count) => total + count, 0) }}</strong> <span v-for="(count, kind) in hitGapCounts" :key="kind">{{ gapLabel(kind) }}: {{ count }}; </span></div>
      <div v-if="result.gaps?.length"><strong>{{ messages.gaps }}:</strong> <span v-for="(gap, index) in result.gaps" :key="index">{{ gap.message || gap.kind }}{{ index < result.gaps.length - 1 ? '; ' : '' }}</span></div>
    </div>
    <p v-if="result.abstain">{{ messages.searchAbstain }}</p>
    <template v-else>
      <template v-for="(hit, index) in display" :key="`${hit.handle}-${index}`">
      <p v-if="hit.in_filter === false && index === inside.length">{{ messages.searchOutside }}: {{ result.outside_filter_count }}</p>
      <article class="search-card">
        <div v-if="hit.doc" class="document-identity">
          <span v-if="result.documents?.[hit.doc]?.number" class="document-number">{{ result.documents[hit.doc]?.number }}</span>
          <span v-if="result.documents?.[hit.doc]?.title">{{ result.documents[hit.doc]?.title }}</span>
          <span v-if="!result.documents?.[hit.doc]">{{ hit.doc }}</span>
        </div>
        <h2>{{ hit.citation }}</h2>
        <span :class="`stamp ink-${validityInk(hit.doc ? result.documents?.[hit.doc]?.status.class : undefined)}`">{{ label(hit.doc ? result.documents?.[hit.doc]?.status.class : undefined) }}</span>
        <ul v-if="hit.gaps?.length" class="hit-gaps"><li v-for="(gap, i) in hit.gaps" :key="i">{{ gapLabel(gap.kind) }}</li></ul>
        <p class="law-text">{{ hit.text ?? hit.snippet }}</p>
        <div class="relations">
          <span v-for="(relation, i) in [...(hit.warnings || []), ...(result.documents?.[hit.doc || '']?.related || []), ...(hit.relations || [])]" :key="i" class="relation-chip">{{ 'dir' in relation ? relation.dir : '' }} {{ relation.kind }} {{ 'number' in relation ? relation.number : '' }}</span>
        </div>
        <div class="nodes"><template v-for="code in hit.nodes" :key="code"><NuxtLink v-if="result.nodes?.[code]?.name" class="relation-chip" :to="`/vn/topics/${code}`">{{ result.nodes[code]?.name }}</NuxtLink></template></div>
        <NuxtLink v-if="hit.doc && result.release && readerLink(hit, result.release)" :to="readerLink(hit, result.release)!">{{ messages.searchRead }}</NuxtLink>
      </article>
      </template>
      <p v-if="!hits.length">{{ messages.searchEmpty }}</p>
    </template>
  </section>
</template>
