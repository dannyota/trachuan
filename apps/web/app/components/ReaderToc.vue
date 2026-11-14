<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import type { ReaderPosition } from '../utils/reader-position'
import { displayHeading } from '../utils/reader-units'
import { readerMessages } from '../../i18n/reader'
import type { Language } from '../../i18n'
const props = defineProps<{ units: components['schemas']['Unit'][], current?: string, position?: ReaderPosition }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
const nav = ref<HTMLElement>()
let selected: HTMLElement[] = []
let entries = new Map<string, { link: HTMLElement, chapter?: HTMLElement }>()
function level(unit: components['schemas']['Unit']) {
  if (/^(phần|chương)(\s|$)/i.test(unit.label || '')) return 'reader-toc-chapter'
  if (/^mục(\s|$)/i.test(unit.label || '')) return 'reader-toc-section'
  return 'reader-toc-article'
}
function update() {
  const anchor = (props.position?.anchor.value ?? props.current ?? '').split('/')[0] || ''
  const entry = entries.get(anchor)
  const next = entry ? [entry.chapter, entry.link].filter((item): item is HTMLElement => !!item) : []
  const container = entry && nav.value?.clientHeight ? nav.value.parentElement : null
  let scrollTo: number | undefined
  if (entry && container && container.scrollHeight > container.clientHeight) {
    const linkRect = entry.link.getBoundingClientRect()
    const top = linkRect.top - container.getBoundingClientRect().top + container.scrollTop
    if (top < container.scrollTop) scrollTo = top
    else if (top + linkRect.height > container.scrollTop + container.clientHeight) scrollTo = top + linkRect.height - container.clientHeight
  }
  for (const item of selected) if (!next.includes(item)) item.removeAttribute('aria-current')
  for (const item of next) if (!selected.includes(item)) item.setAttribute('aria-current', 'location')
  selected = next
  if (container && scrollTo !== undefined) container.scrollTop = scrollTo
}
function indexEntries() {
  for (const item of selected) item.removeAttribute('aria-current')
  entries = new Map()
  let chapter: HTMLElement | undefined
  const links = nav.value?.querySelectorAll<HTMLElement>('[data-toc-anchor]') || []
  for (const link of links) {
    if (link.parentElement?.classList.contains('reader-toc-chapter')) chapter = link
    const anchor = link.dataset.tocAnchor
    if (anchor) entries.set(anchor, { link, chapter: chapter === link ? undefined : chapter })
  }
  selected = []
  update()
}
onMounted(indexEntries)
watch(() => props.units, indexEntries, { flush: 'post' })
watch(() => props.position?.anchor.value ?? props.current, update)
</script>
<template>
  <nav ref="nav" :aria-label="m.contents" class="reader-toc">
    <ul><li v-for="(unit, index) in units" :key="index" v-memo="[unit]" :class="level(unit)"><a v-if="unit.anchor" :href="`#${encodeURIComponent(unit.anchor)}`" :data-toc-anchor="unit.anchor">{{ displayHeading(unit.label, unit.heading) }}</a><span v-else>{{ displayHeading(unit.label, unit.heading) }}</span></li></ul>
  </nav>
</template>
