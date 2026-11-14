<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { documentCitation, warningSentence } from '../content'
import { displayHeading } from '../utils/reader-units'
import { readerMessages } from '../../i18n/reader'
import { messagesFor, type Language } from '../../i18n'
import CopyButton from './CopyButton.vue'
import { warningLink } from '../utils/search-results'
const props = defineProps<{ unit: components['schemas']['Unit'], document?: components['schemas']['Document'], target: boolean, release: string, warnings: components['schemas']['Warning'][] }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
const heading = computed(() => /^(khoản|điểm)\b/i.test(props.unit.label || '') ? '' : displayHeading(props.unit.label, props.unit.heading))
const text = computed(() => {
  const source = props.unit.text || ''
  if (!heading.value || !props.unit.label || !props.unit.heading) return source
  const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').trim().split(/\s+/).join('\\s+')
  const pattern = new RegExp(`^\\s*${escape(props.unit.label)}\\.?\\s+${escape(props.unit.heading)}(?=\\s|$)\\s*`)
  return source.replace(pattern, '')
})
const citation = computed(() => props.document ? documentCitation(props.unit.anchor, props.document) : null)
</script>
<template>
  <article :id="unit.anchor" class="reader-unit" :data-anchor="unit.anchor" tabindex="-1">
    <h2 v-if="heading" :data-spy="/^(chương|điều)(\s|$)/i.test(unit.label || '') ? 'true' : undefined" :data-spy-anchor="unit.anchor">{{ heading }}</h2>
    <div v-if="target" class="reader-target-label"><strong>{{ unit.anchor?.includes('/diem-') ? m.targetPoint : unit.anchor?.includes('/khoan-') ? m.targetClause : m.targetArticle }}</strong><CopyButton v-if="citation" :text="citation" :label="`${m.copy} ${citation}`" /></div>
    <p v-for="(warning, i) in warnings" :key="i" class="notice notice--warn"><NuxtLink v-if="warningLink(warning, release)" :to="warningLink(warning, release)!">{{ warningSentence(warning, language) }}</NuxtLink><template v-else>{{ warningSentence(warning, language) }}</template></p>
    <p v-if="unit.warnings_omitted" class="notice notice--warn">{{ messagesFor(language).warningMore.replace('{n}', String(unit.warnings_omitted)) }}</p>
    <p v-for="(line, index) in text ? text.split('\n') : []" :key="index" class="law-text" :class="{ 'reader-point': /^\s*[a-zđ]\)/i.test(line) }">{{ line || '\u00a0' }}</p>
  </article>
</template>
