<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { readerMessages } from '../../i18n/reader'
import type { Language } from '../../i18n'
const props = defineProps<{ units: components['schemas']['Unit'][], current: string }>()
const selected = computed(() => props.units.map(unit => unit.anchor).filter((anchor): anchor is string => !!anchor && (props.current === anchor || props.current.startsWith(`${anchor}/`))).sort((a, b) => b.length - a.length)[0])
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
function level(unit: components['schemas']['Unit']) {
  if (/^(phần|chương)\b/i.test(unit.label || '')) return 'reader-toc-chapter'
  if (/^mục\b/i.test(unit.label || '')) return 'reader-toc-section'
  return 'reader-toc-article'
}
</script>
<template>
  <nav :aria-label="m.contents" class="reader-toc">
    <ul><li v-for="(unit, index) in units" :key="index" :class="level(unit)"><a v-if="unit.anchor" :href="`#${encodeURIComponent(unit.anchor)}`" :aria-current="unit.anchor === selected ? 'location' : undefined">{{ unit.label }} {{ unit.heading }}</a><span v-else>{{ unit.label }} {{ unit.heading }}</span></li></ul>
  </nav>
</template>
