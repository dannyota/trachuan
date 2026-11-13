<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
import { formatDate, formatNumber, formatRelease, gapSummary } from '../content'

const props = defineProps<{ status: components['schemas']['CorpusStatus'] | null, showGaps?: boolean }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => pagesMessages(language.value))
const gaps = computed(() => props.status ? gapSummary(props.status, language.value) : [])
</script>
<template>
  <section class="coverage-facts" :aria-label="messages.coverage">
    <p v-if="!status">{{ messages.coverageUnavailable }}</p>
    <template v-else>
      <dl class="coverage-stats">
        <div><dt>{{ labels.documents }}</dt><dd>{{ formatNumber(status.documents, language) }}</dd></div>
        <div><dt>{{ labels.provisions }}</dt><dd>{{ formatNumber(status.provisions, language) }}</dd></div>
        <div><dt>{{ labels.nodes }}</dt><dd>{{ formatNumber(status.nodes, language) }}</dd></div>
        <div><dt>{{ labels.newest }}</dt><dd>{{ status.newest_source_date ? formatDate(status.newest_source_date, language) : '-' }}</dd></div>
      </dl>
      <p class="coverage-release">{{ labels.release }}: {{ formatRelease(status.release, language) }}</p>
    </template>
    <slot />
    <section v-if="showGaps && status" class="page-section"><h2>{{ labels.missing }}</h2><ul class="gap-list"><li v-for="gap in gaps" :key="gap.key"><h3>{{ gap.name }}</h3><p>{{ gap.sentence }} {{ gap.casesLabel }}</p></li></ul></section>
  </section>
</template>
