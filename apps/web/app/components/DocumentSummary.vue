<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { formatDate, formatNumber, gapCategory, relationLabel, statusBasisLabel } from '../content'
import type { Language } from '../../i18n'
import { readerMessages } from '../../i18n/reader'
import { summaryMessages } from '../../i18n/summary'
import { isDocumentKey } from '../utils/document-key'

defineProps<{ summary: components['schemas']['PublicSummary'] }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => summaryMessages(language.value))
const labels = computed(() => readerMessages(language.value))
function source(value: string | null) { try { return value && new URL(value).protocol === 'https:' ? value : undefined } catch { return undefined } }
</script>

<template>
  <article class="document-summary">
    <header class="page-intro">
      <div class="summary-status"><ValidityStamp :status="summary.document.status.class" /><span>{{ statusBasisLabel(summary.document.status, summary.document.source_url ?? undefined, language) }}</span></div>
      <p>{{ m.statusDate }} <time :datetime="summary.status_date">{{ formatDate(summary.status_date, language) }}</time></p>
      <p class="num">{{ summary.document.number }}</p>
      <h1>{{ summary.document.title }}</h1>
      <p>{{ summary.document.type }} · {{ summary.document.issuer }}</p>
      <p v-if="summary.document.issued">{{ labels.issued }} <time :datetime="summary.document.issued">{{ formatDate(summary.document.issued, language) }}</time></p>
      <p v-if="summary.document.effective_from">{{ labels.effectiveFrom }} <time :datetime="summary.document.effective_from">{{ formatDate(summary.document.effective_from, language) }}</time></p>
      <p v-if="summary.document.effective_to">{{ labels.effectiveTo }} <time :datetime="summary.document.effective_to">{{ formatDate(summary.document.effective_to, language) }}</time></p>
    </header>
    <p v-if="summary.document.status.unresolved" class="notice notice--warn">{{ m.unresolved }}</p>
    <p v-for="kind in summary.document.gap_kinds" :key="kind" class="notice notice--warn">{{ gapCategory(kind, language).name }}</p>
    <p v-if="!summary.document.has_text" class="notice">{{ m.missing }}</p>
    <template v-else>
      <p v-if="!summary.document.text_complete" class="notice">{{ m.incomplete }}</p>
      <NuxtLink class="btn btn-primary summary-read" :to="`/vn/documents/${summary.document.key}`">{{ m.read }}</NuxtLink>
    </template>
    <section v-if="summary.outline.length" class="page-section"><h2>{{ labels.contents }}</h2>
      <ol class="summary-outline" role="list"><li v-for="(entry, index) in summary.outline" :key="index" :data-kind="entry.kind"><span>{{ entry.label }}</span><template v-if="entry.heading">. {{ entry.heading }}</template></li></ol>
    </section>
    <section v-if="summary.relations_total" class="page-section"><h2>{{ labels.relations }}</h2>
      <p>{{ m.relationsTotal }}: {{ formatNumber(summary.relations_total, language) }}</p>
      <ul><li v-for="(relation, index) in summary.relations" :key="index">{{ relationLabel(relation.kind, relation.direction === 'incoming' ? 'in' : 'out', language) }} <NuxtLink v-if="relation.key && isDocumentKey(relation.key)" :to="`/vn/documents/${relation.key}/summary`"><span class="num">{{ relation.number }}</span><template v-if="relation.title">{{ ' ' }}<span class="summary-title">{{ relation.title }}</span></template></NuxtLink><template v-else><span class="num">{{ relation.number }}</span><template v-if="relation.title">{{ ' ' }}<span class="summary-title">{{ relation.title }}</span></template></template></li></ul>
    </section>
    <section v-if="summary.nodes.length" class="page-section"><h2>{{ labels.topics }}</h2><ul><li v-for="node in summary.nodes" :key="node.code"><NuxtLink :to="`/vn/topics/${node.code}`">{{ node.code }} · {{ node.name }}</NuxtLink></li></ul></section>
    <p v-if="source(summary.document.source_url)" class="page-section">{{ labels.sourceUrl }}: <a :href="source(summary.document.source_url)" rel="noopener noreferrer">{{ summary.document.source_url }}</a></p>
  </article>
</template>

<style scoped>
.summary-read { margin-block-start: 8px; }
.document-summary { max-width: 52rem; margin-inline: auto; }
.document-summary h1, .summary-outline, .summary-title { font-family: var(--font-serif); }
.summary-status { display: flex; flex-wrap: wrap; align-items: center; gap: 1rem; }
.summary-outline { list-style: none; padding: 0; }
.summary-outline li { margin-block: 0.75rem; }
.summary-outline [data-kind="muc"], .summary-outline [data-kind="dieu"] { padding-inline-start: 1rem; }
</style>
