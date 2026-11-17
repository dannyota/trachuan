<script setup lang="ts">
import { computed } from 'vue'
import { Lock } from '@lucide/vue'
import type { components } from '@trachuan/schema/gen/openapi'
import { frameworkMessages } from '../../i18n/frameworks'
import type { Language } from '../../i18n'
import { controlPath, versionPath } from '../utils/framework-handles'
import FrameworkAccess from './FrameworkAccess.vue'

type Unit = components['schemas']['FrameworkReadUnit']
type End = components['schemas']['FrameworkMappingEnd']
const props = defineProps<{ units: Unit[], handle: string }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => frameworkMessages(language.value))
const units = computed(() => {
  const joined: Unit[] = []
  for (const unit of props.units) {
    const last = joined.at(-1)
    if (unit.part && unit.part > 1 && last && last.kind === unit.kind) {
      if (unit.kind === 'mapping' && last.edge && unit.edge) last.edge = { ...last.edge, evidence: (last.edge.evidence || '') + (unit.edge.evidence || '') }
      else last.text = (last.text || '') + (unit.text || '')
    } else joined.push({ ...unit })
  }
  return joined
})
function endPath(end: End) { return end.handle ? controlPath(end.handle, end.citation) : undefined }
</script>
<template>
  <div>
    <template v-for="(unit, index) in units" :key="index">
      <template v-if="unit.head">
        <header class="reader-identity fw-identity">
          <div class="container reader-identity-inner">
            <span class="num fw-mono">{{ unit.head.citation }}</span>
            <h1 :aria-label="unit.head.title ? `${unit.head.citation} · ${unit.head.title}` : undefined">{{ unit.head.title || `${unit.head.citation} · ${unit.head.kind}` }}</h1>
            <p class="fw-meta"><span>{{ [unit.head.kind, unit.head.status].filter(Boolean).join(' · ') }} · </span><FrameworkAccess :access="unit.head.access" /><span> · {{ unit.head.text_withheld ? labels.unreadable : labels.readable }}</span></p>
            <nav v-if="unit.head.path?.length"><NuxtLink v-for="ancestor in unit.head.path" :key="ancestor" :to="controlPath(handle, ancestor)" class="fw-mono fw-ancestor">{{ ancestor }}</NuxtLink></nav>
            <p v-for="(target, key) in unit.head.lineage" :key="key" class="fw-meta"><NuxtLink :to="target ? versionPath(target) : undefined" class="fw-mono">{{ target }}</NuxtLink></p>
          </div>
        </header>
        <div class="container fw-content"><slot name="notices" /><p v-if="unit.head.text_withheld" class="notice fw-withheld"><Lock :size="18" aria-hidden="true" /><span>{{ labels.withheld }}</span></p></div>
      </template>
      <template v-else-if="unit.version"><header class="reader-identity fw-identity">
        <div class="container reader-identity-inner">
          <span class="num fw-mono">{{ unit.version.code }} {{ unit.version.version }}</span>
          <h1>{{ unit.version.name }}</h1>
          <p class="fw-meta"><span v-if="unit.version.publisher">{{ unit.version.publisher }} · </span><FrameworkAccess :access="unit.version.access" /><span> · {{ unit.version.readable ? labels.readable : labels.unreadable }}</span></p>
          <p v-if="!unit.version.readable" class="fw-meta">{{ labels.account }}</p>
          <p v-for="(target, key) in unit.version.lineage" :key="key" class="fw-meta"><NuxtLink :to="target ? versionPath(target) : undefined" class="fw-mono">{{ target }}</NuxtLink></p>
        </div>
      </header><div class="container fw-content"><slot name="notices" /></div></template>
      <div v-else-if="['title_original', 'body', 'note', 'table'].includes(unit.kind)" class="container fw-content"><p class="fw-source">{{ unit.text }}</p></div>
      <div v-else-if="unit.edge" class="container fw-content">
        <h2 v-if="index === 0 || units[index - 1]?.kind !== 'mapping'" class="fw-section-title">{{ labels.mappings }}</h2>
        <div class="fw-row"><div v-for="(end, key) in [unit.edge.from, unit.edge.to]" :key="key"><p class="fw-meta"><NuxtLink v-if="endPath(end)" :to="endPath(end)" class="fw-mono">{{ end.citation }}</NuxtLink><span v-else class="fw-mono">{{ end.code }} {{ end.citation }}</span> · {{ end.version }}</p><p v-if="end.handle && end.title" class="fw-title">{{ end.title }}</p></div><p class="fw-meta">{{ labels.relation[unit.edge.relation] }} · {{ unit.edge.source }}</p><p v-if="unit.edge.evidence" class="fw-source">{{ unit.edge.evidence }}</p></div>
      </div>
      <div v-else-if="unit.outline" class="container fw-content">
        <h2 v-if="index === 0 || units[index - 1]?.kind !== 'outline'" class="fw-section-title">{{ labels.children }}</h2>
        <div class="fw-row"><NuxtLink :to="controlPath(handle, unit.outline.citation)" class="fw-mono">{{ unit.outline.citation }}</NuxtLink><p v-if="unit.outline.title" class="fw-title">{{ unit.outline.title }}</p><p v-if="unit.outline.kind" class="fw-meta">{{ unit.outline.kind }}</p></div>
      </div>
    </template>
  </div>
</template>
<style scoped>
.fw-mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.fw-source { font-family: var(--font-serif); white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
