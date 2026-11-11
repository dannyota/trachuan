<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, statusMessage, type Language } from '../../i18n'
import { readerLink, resultHits, validityInk } from '../utils/search-results'

const props = defineProps<{ result: components['schemas']['SearchResult'] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const hits = computed(() => resultHits(props.result))
const inks = ['in-force', 'partial', 'not-yet', 'expired', 'neutral'] as const
const counts = computed(() => Object.fromEntries(inks.map(ink => [ink, hits.value.filter(hit => validityInk(hit.doc ? props.result.documents?.[hit.doc]?.status.class : undefined) === ink).length])))
function label(status: string | undefined) {
  return validityInk(status) === 'neutral' && status ? status : statusMessage(language.value, status)
}
</script>

<template>
  <section class="search-results" aria-live="polite">
    <div class="search-summary">
      <span v-for="(status, index) in ['in_force', 'partial', 'not_yet', 'expired', 'unknown']" :key="status" :class="`ink-${inks[index]}`">{{ label(status) }}: {{ counts[inks[index]!] }}</span>
      <div v-if="result.gaps?.length"><strong>{{ messages.gaps }}:</strong> <span v-for="(gap, index) in result.gaps" :key="index">{{ gap.message || gap.kind }}{{ index < result.gaps.length - 1 ? '; ' : '' }}</span></div>
    </div>
    <p v-if="result.abstain">{{ messages.searchAbstain }}</p>
    <template v-else>
      <article v-for="(hit, index) in hits" :key="`${hit.handle}-${index}`" class="search-card">
        <h2>{{ hit.citation }}</h2>
        <span :class="`stamp ink-${validityInk(hit.doc ? result.documents?.[hit.doc]?.status.class : undefined)}`">{{ label(hit.doc ? result.documents?.[hit.doc]?.status.class : undefined) }}</span>
        <p class="law-text">{{ hit.text ?? hit.snippet }}</p>
        <div class="relations">
          <span v-for="(relation, i) in [...(hit.warnings || []), ...(result.documents?.[hit.doc || '']?.related || []), ...(hit.relations || [])]" :key="i" class="relation-chip">{{ 'dir' in relation ? relation.dir : '' }} {{ relation.kind }} {{ 'number' in relation ? relation.number : '' }}</span>
        </div>
        <NuxtLink v-if="hit.doc && result.release" :to="readerLink(hit, result.release)">{{ messages.searchRead }}</NuxtLink>
      </article>
      <p v-if="!hits.length">{{ messages.searchEmpty }}</p>
    </template>
  </section>
</template>
