<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { readerMessages } from '../../i18n/reader'
import type { Language } from '../../i18n'
defineProps<{ units: components['schemas']['Unit'][], current: string }>()
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
    <ul><li v-for="(unit, index) in units" :key="index" :class="level(unit)"><a v-if="unit.anchor" :href="`#${encodeURIComponent(unit.anchor)}`" :aria-current="unit.anchor === current ? 'location' : undefined">{{ unit.label }} {{ unit.heading }}</a><span v-else>{{ unit.label }} {{ unit.heading }}</span></li></ul>
  </nav>
</template>
