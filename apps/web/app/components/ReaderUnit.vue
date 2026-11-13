<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { documentCitation, warningSentence } from '../content'
import { readerMessages } from '../../i18n/reader'
import { messagesFor, type Language } from '../../i18n'
import CopyButton from './CopyButton.vue'
import { warningLink } from '../utils/search-results'
const props = defineProps<{ unit: components['schemas']['Unit'], document?: components['schemas']['Document'], target: boolean, release: string, warnings: components['schemas']['Warning'][] }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
const citation = computed(() => props.document ? documentCitation(props.unit.anchor, props.document) : null)
</script>
<template>
  <article :id="unit.anchor" class="reader-unit" :class="{ 'reader-unit-target': target }" :data-anchor="unit.anchor">
    <h2 v-if="unit.label || unit.heading">{{ unit.label }} {{ unit.heading }}</h2>
    <div v-if="target" class="reader-target-label"><strong>{{ m.target }}</strong><CopyButton v-if="citation" :text="citation" :label="`${m.copy} ${citation}`" /></div>
    <p v-for="(warning, i) in warnings" :key="i" class="notice notice--warn"><NuxtLink v-if="warningLink(warning, release)" :to="warningLink(warning, release)!">{{ warningSentence(warning, language) }}</NuxtLink><template v-else>{{ warningSentence(warning, language) }}</template></p>
    <p v-if="unit.warnings_omitted" class="notice notice--warn">{{ messagesFor(language).warningMore.replace('{n}', String(unit.warnings_omitted)) }}</p>
    <p v-for="(line, index) in unit.text?.split('\n') || []" :key="index" class="law-text" :class="{ 'reader-point': /^\s*[a-zđ]\)/i.test(line) }">{{ line || '\u00a0' }}</p>
  </article>
</template>
