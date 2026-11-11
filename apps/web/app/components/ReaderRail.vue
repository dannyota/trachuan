<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, statusMessage, type Language } from '../../i18n'
import { isDocumentKey } from '../utils/document-key'
import { validityInk } from '../utils/search-results'

defineProps<{ document?: components['schemas']['Document'], gaps: components['schemas']['Gap'][], release: string, hash?: string }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
function safeSource(value?: string) {
  try { return value && new URL(value).protocol === 'https:' ? value : undefined } catch { return undefined }
}
function gapLabel(kind?: string) {
  switch (kind) {
    case 'validity_conflict': return messages.value.gapValidityConflict
    case 'missing_effective_date': return messages.value.gapMissingEffectiveDate
    case 'consolidated_status_unordered': return messages.value.gapConsolidatedUnordered
    case 'consolidated_status_unresolved': return messages.value.gapConsolidatedUnresolved
    case 'ambiguous_tree_text': return messages.value.gapAmbiguousText
    default: return kind || messages.value.statusUnknown
  }
}
</script>

<template>
  <aside class="reader-rail">
    <template v-if="document">
      <span :class="`stamp ink-${validityInk(document.status.class)}`">{{ statusMessage(language, document.status.class) }}</span>
      <p class="document-number">{{ document.number }}</p>
      <h1>{{ document.title }}</h1>
      <p v-if="document.effective_from">{{ messages.readerEffectiveFrom }}: {{ document.effective_from }}</p>
      <p v-if="document.effective_to">{{ messages.readerEffectiveTo }}: {{ document.effective_to }}</p>
      <div v-if="document.related_counts && Object.keys(document.related_counts).length">
        <h2>{{ messages.readerRelated }}</h2>
        <p v-for="(count, kind) in document.related_counts" :key="kind">{{ kind }}: {{ count }}</p>
      </div>
      <ul v-if="document.related?.length">
        <li v-for="(relation, index) in document.related" :key="index">
          {{ relation.dir }} {{ relation.kind }}
          <NuxtLink v-if="relation.key && isDocumentKey(relation.key)" :to="`/vn/documents/${relation.key}?release=${encodeURIComponent(release)}`">{{ relation.number || relation.key }}</NuxtLink>
          <span v-else>{{ relation.number || relation.key }}</span>
        </li>
      </ul>
      <p v-if="document.source_url"><a v-if="safeSource(document.source_url)" :href="safeSource(document.source_url)" rel="noopener noreferrer">{{ document.source_url }}</a><span v-else>{{ document.source_url }}</span></p>
    </template>
    <div v-if="gaps.length"><h2>{{ messages.gaps }}</h2><ul><li v-for="(gap, index) in gaps" :key="index">{{ gapLabel(gap.kind) }}</li></ul></div>
    <p>{{ messages.release }}: <span class="document-number">{{ release }}</span></p>
    <p v-if="hash">SHA-256: <span class="document-number">{{ hash }}</span></p>
  </aside>
</template>
