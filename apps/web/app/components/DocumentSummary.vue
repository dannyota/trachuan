<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { formatDate, formatNumber, gapCategory, relationLabel, statusBasisLabel } from '../content'
import type { Language } from '../../i18n'
import { readerMessages } from '../../i18n/reader'
import { summaryMessages } from '../../i18n/summary'
import { isDocumentKey } from '../utils/document-key'

const props = defineProps<{ summary: components['schemas']['PublicSummary'] }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => summaryMessages(language.value))
const labels = computed(() => readerMessages(language.value))
const dates = computed(() => [
  { label: labels.value.issued, value: props.summary.document.issued },
  { label: labels.value.effectiveFrom, value: props.summary.document.effective_from },
  { label: labels.value.effectiveTo, value: props.summary.document.effective_to },
].flatMap(date => date.value ? [{ label: date.label, value: date.value }] : []))
function source(value: string | null) { try { return value && new URL(value).protocol === 'https:' ? value : undefined } catch { return undefined } }
</script>

<template>
  <article class="document-summary">
    <header class="reader-identity">
      <div class="container reader-identity-inner">
        <div class="reader-identity-line"><ValidityStamp :status="summary.document.status.class" /><span>{{ statusBasisLabel(summary.document.status, summary.document.source_url ?? undefined, language) }}</span><span class="num">{{ summary.document.number }}</span><span>{{ [summary.document.type, summary.document.issuer].filter(Boolean).join(' · ') }}</span></div>
        <h1>{{ summary.document.title }}</h1>
        <p v-if="dates.length" class="reader-meta summary-dates"><template v-for="(date, index) in dates" :key="date.label"><template v-if="index">{{ ' · ' }}</template><span>{{ date.label }} <strong><time :datetime="date.value">{{ formatDate(date.value, language) }}</time></strong></span></template></p>
        <p class="reader-meta summary-status-date"><span>{{ m.statusDate }} <time :datetime="summary.status_date">{{ formatDate(summary.status_date, language) }}</time></span></p>
        <p v-if="summary.document.status.unresolved" class="notice notice--warn">{{ m.unresolved }}</p>
        <p v-for="kind in summary.document.gap_kinds" :key="kind" class="notice notice--warn">{{ gapCategory(kind, language).name }}</p>
        <p v-if="!summary.document.has_text" class="notice">{{ m.missing }}</p>
        <template v-else>
          <p v-if="!summary.document.text_complete" class="notice">{{ m.incomplete }}</p>
          <div class="reader-actions"><NuxtLink class="btn btn-primary summary-read" :to="`/vn/documents/${summary.document.key}`">{{ m.read }}</NuxtLink></div>
        </template>
      </div>
    </header>
    <div class="container reader"><div class="reader-content summary-content">
    <section v-if="summary.outline.length" class="page-section"><h2>{{ labels.contents }}</h2>
      <ol class="summary-outline" role="list"><li v-for="(entry, index) in summary.outline" :key="index" :data-kind="entry.kind"><span>{{ entry.label }}</span><template v-if="entry.heading">. {{ entry.heading }}</template></li></ol>
    </section>
    <section v-if="summary.relations_total" class="page-section"><h2>{{ labels.relations }}</h2>
      <p>{{ m.relationsTotal }}: {{ formatNumber(summary.relations_total, language) }}</p>
      <ul><li v-for="(relation, index) in summary.relations" :key="index">{{ relationLabel(relation.kind, relation.direction === 'incoming' ? 'in' : 'out', language) }} <NuxtLink v-if="relation.key && isDocumentKey(relation.key)" :to="`/vn/documents/${relation.key}/summary`"><span class="num">{{ relation.number }}</span><template v-if="relation.title">{{ ' ' }}<span class="summary-title">{{ relation.title }}</span></template></NuxtLink><template v-else><span class="num">{{ relation.number }}</span><template v-if="relation.title">{{ ' ' }}<span class="summary-title">{{ relation.title }}</span></template></template></li></ul>
    </section>
    <section v-if="summary.nodes.length" class="page-section"><h2>{{ labels.topics }}</h2><ul><li v-for="node in summary.nodes" :key="node.code"><NuxtLink :to="`/vn/topics/${node.code}`">{{ node.code }} · {{ node.name }}</NuxtLink></li></ul></section>
    <p v-if="source(summary.document.source_url)" class="page-section">{{ labels.sourceUrl }}: <a :href="source(summary.document.source_url)" rel="noopener noreferrer">{{ summary.document.source_url }}</a></p>
    </div></div>
  </article>
</template>

<style scoped>
.reader-identity-inner { overflow-wrap: anywhere; }
.reader-identity-inner p { margin: 0; }
.summary-dates { display: block; }
.summary-dates span { white-space: nowrap; }
.summary-content { grid-column: 2; }
.summary-content > .page-section:first-child { margin-top: 0; }
.summary-outline, .summary-title { font-family: var(--font-serif); }
.summary-outline { list-style: none; padding: 0; }
.summary-outline li { margin-block: 0.75rem; }
.summary-outline [data-kind="muc"], .summary-outline [data-kind="dieu"] { padding-inline-start: 1rem; }
@media (max-width: 1023px) { .summary-content { grid-column: 1; } }
</style>
