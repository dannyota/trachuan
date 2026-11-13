<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { formatRelease, gapCategory, relationCountLabel, warningSentence } from '../content'
import { messagesFor, type Language } from '../../i18n'
import { readerMessages } from '../../i18n/reader'
import RelationsPanel from './RelationsPanel.vue'
import { warningLink } from '../utils/search-results'

defineProps<{ document?: components['schemas']['Document'], gaps: components['schemas']['Gap'][], release: string, hash?: string, nodes?: components['schemas']['NodeIndex'], units?: components['schemas']['Unit'][], documentKey?: string, warnings?: Map<string, components['schemas']['Warning'][]> }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
function safeSource(value?: string) { try { return value && new URL(value).protocol === 'https:' ? value : undefined } catch { return undefined } }
</script>
<template>
  <aside class="reader-rail" :aria-label="m.technical">
    <RelationsPanel v-if="document && documentKey" :key="`${documentKey}:${release}`" :document="document" :document-key="documentKey" :release="release" />
    <section v-if="document?.nodes?.some(code => nodes?.[code]?.name)" class="card reader-card"><h2>{{ m.topics }}</h2><div class="reader-chips"><template v-for="code in document.nodes" :key="code"><NuxtLink v-if="nodes?.[code]?.name" class="chip" :to="`/vn/topics/${code}`">{{ nodes[code]?.name }}</NuxtLink></template></div></section>
    <section v-if="units?.some(unit => warnings?.get(`${documentKey}#${unit.anchor}`)?.length || unit.warnings_omitted)" class="card reader-card reader-warnings"><h2>{{ m.warnings }}</h2><div v-for="(unit, index) in units" :key="index"><template v-if="warnings?.get(`${documentKey}#${unit.anchor}`)?.length || unit.warnings_omitted"><h3>{{ warnings?.get(`${documentKey}#${unit.anchor}`)?.[0]?.citation || unit.citation || unit.label }}</h3><p v-for="(warning, i) in warnings?.get(`${documentKey}#${unit.anchor}`)" :key="i" class="notice notice--warn"><NuxtLink v-if="warningLink(warning, release)" :to="warningLink(warning, release)!">{{ warningSentence(warning, language) }}</NuxtLink><template v-else>{{ warningSentence(warning, language) }}</template></p><p v-if="unit.warnings_omitted" class="notice notice--warn">{{ messagesFor(language).warningMore.replace('{n}', String(unit.warnings_omitted)) }}</p></template></div></section>
    <details class="card reader-card"><summary>{{ m.technical }}</summary><div class="reader-technical"><p>{{ m.release }}: {{ formatRelease(release, language) }}</p><p v-if="hash">SHA-256: <span class="num">{{ hash }}</span></p><p v-for="(gap, index) in gaps" :key="index">{{ gapCategory(gap.kind, language).name }}</p><p v-if="document?.source_url">{{ m.sourceUrl }}: <a v-if="safeSource(document.source_url)" :href="safeSource(document.source_url)" rel="noopener noreferrer">{{ document.source_url }}</a><span v-else>{{ document.source_url }}</span></p><p v-for="(count, name) in document?.related_counts" :key="name">{{ relationCountLabel(String(name), language) }}: {{ count }}</p></div></details>
  </aside>
</template>
