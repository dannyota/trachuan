<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
import { formatDate, formatNumber, formatRelease, gapSummary } from '../content'

const props = withDefaults(defineProps<{ status: components['schemas']['CorpusStatus'] | null, showGaps?: boolean, showRelease?: boolean, sourceSentence?: boolean }>(), { showRelease: true })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => pagesMessages(language.value))
const releaseSentence = computed(() => {
  const status = props.status
  if (!status) return ''
  const release = formatRelease(status.release, language.value)
  const documents = formatNumber(status.documents, language.value)
  const provisions = formatNumber(status.provisions, language.value)
  const date = status.newest_source_date ? formatDate(status.newest_source_date, language.value) : ''
  return language.value === 'vi' ? `Bản phát hành ${release} có ${documents} văn bản và ${provisions} quy định${date ? `; nguồn mới nhất ngày ${date}` : ''}.` : `Release ${release} has ${documents} documents and ${provisions} provisions${date ? `; the newest source is dated ${date}` : ''}.`
})
const gaps = computed(() => props.status ? gapSummary(props.status, language.value) : [])
</script>
<template>
  <section class="coverage-facts" :aria-label="messages.coverage">
    <p v-if="!status">{{ messages.coverageUnavailable }}</p>
    <template v-else>
      <div class="coverage-summary"><dl class="coverage-stats" :class="{ 'coverage-stats-three': !status.newest_source_date }">
        <div><dt>{{ labels.documents }}</dt><dd>{{ formatNumber(status.documents, language) }}</dd></div>
        <div><dt>{{ labels.provisions }}</dt><dd>{{ formatNumber(status.provisions, language) }}</dd></div>
        <div><dt>{{ labels.nodes }}</dt><dd>{{ formatNumber(status.nodes, language) }}</dd></div>
        <div v-if="status.newest_source_date"><dt>{{ labels.newest }}</dt><dd>{{ formatDate(status.newest_source_date, language) }}</dd></div>
      </dl>
      <p v-if="showRelease !== false" class="coverage-release">{{ sourceSentence ? releaseSentence : `${labels.release}: ${formatRelease(status.release, language)}` }}</p></div>
    </template>
    <slot />
    <section v-if="showGaps && status" class="page-section"><h2>{{ labels.missing }}</h2><ul class="gap-list"><li v-for="gap in gaps" :key="gap.key"><h3>{{ gap.name }}</h3><p>{{ gap.casesSentence }}</p></li></ul></section>
  </section>
</template>
