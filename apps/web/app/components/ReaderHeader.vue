<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { formatDate, formatRelease, relationLabel, statusBasisLabel, textSourceLabel } from '../content'
import { readerMessages } from '../../i18n/reader'
import type { Language } from '../../i18n'
import { isDocumentKey } from '../utils/document-key'
import ValidityStamp from './ValidityStamp.vue'
import CopyButton from './CopyButton.vue'
import ReaderBack from './ReaderBack.vue'

const props = defineProps<{ document: components['schemas']['Document'], release: string }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
const source = computed(() => { try { const url = new URL(props.document.source_url || ''); return url.protocol === 'https:' ? url.href : undefined } catch { return undefined } })
const amendments = computed(() => props.document.related?.filter(r => r.dir === 'in' && r.kind === 'amends_supplements') || [])
const citation = computed(() => [props.document.type, props.document.number].filter(Boolean).join(' '))
</script>
<template>
  <header class="reader-identity">
    <div class="container reader-identity-inner">
      <ReaderBack class="reader-back-mobile" />
      <div class="reader-identity-line">
        <ValidityStamp :status="document.status?.class" />
        <span v-if="statusBasisLabel(document.status, document.source_url, language)">{{ statusBasisLabel(document.status, document.source_url, language) }}</span>
        <span class="num">{{ document.number }}</span>
        <span>{{ [document.type, document.issuer].filter(Boolean).join(' · ') }}</span>
      </div>
      <h1>{{ document.title }}</h1>
      <div class="reader-meta">
        <span v-if="document.issued">{{ m.issued }} <strong>{{ formatDate(document.issued, language) }}</strong></span>
        <span v-if="document.effective_from">{{ m.effectiveFrom }} <strong>{{ formatDate(document.effective_from, language) }}</strong></span>
        <span v-if="document.effective_to">{{ m.effectiveTo }} <strong>{{ formatDate(document.effective_to, language) }}</strong></span>
        <span v-if="amendments.length">{{ relationLabel('amends_supplements', 'in', language) }} <template v-for="(item, i) in amendments" :key="i"><NuxtLink v-if="item.in_release && item.key && isDocumentKey(item.key)" class="num" :to="{ path: `/vn/documents/${item.key}`, query: { release } }">{{ item.number }}</NuxtLink><span v-else class="num">{{ item.number }}</span> </template></span>
        <span>{{ m.release }} {{ formatRelease(release, language) }}</span>
        <span>{{ textSourceLabel(document.text_source, document.text_complete, document.source_url, language) }}</span>
      </div>
      <div class="reader-actions"><a v-if="source" class="btn btn-secondary" :href="source" rel="noopener noreferrer">{{ m.source }}</a><CopyButton v-if="citation" :text="citation" :label="m.copyDocument" /></div>
    </div>
  </header>
</template>
