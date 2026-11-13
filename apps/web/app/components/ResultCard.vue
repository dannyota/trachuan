<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { citationFromAnchor, documentCitation, gapCategory, highlightSpans, hitByline, matchedMembers, warningAdvice, warningSentence } from '../content'
import { readerLink, warningLink } from '../utils/search-results'
import { searchMessages } from '../../i18n/search'
import type { Language } from '../../i18n'
import ValidityStamp from './ValidityStamp.vue'
import CopyButton from './CopyButton.vue'

type Hit = components['schemas']['Hit']
type Result = components['schemas']['SearchResult']
const props = defineProps<{ hit: Hit, documents?: Result['documents'], nodes?: Result['nodes'], release?: string, query?: string, rank?: number }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => searchMessages(language.value))
const doc = computed(() => props.documents?.[props.hit.doc || ''])
const link = computed(() => props.release ? readerLink(props.hit, props.release) : null)
const anchor = computed(() => props.hit.handle?.split('#', 2)[1])
const citation = computed(() => doc.value ? documentCitation(anchor.value, doc.value) : null)
const heading = computed(() => [...(props.hit.path || [])].reverse().find(node => node.heading)?.heading)
const text = computed(() => props.hit.text ?? props.hit.snippet ?? '')
const expanded = ref(false)
const spans = computed(() => highlightSpans(text.value, props.query || ''))
const pieces = computed(() => {
  let position = 0
  const parts: { text: string, marked: boolean }[] = []
  for (const span of spans.value) {
    parts.push({ text: text.value.slice(position, span.start), marked: false }, { text: text.value.slice(span.start, span.end), marked: true })
    position = span.end
  }
  parts.push({ text: text.value.slice(position), marked: false })
  return parts
})
const members = computed(() => {
  const labels = matchedMembers(props.hit)
  const prefix = anchor.value ? `${anchor.value}/` : ''
  const fragments = (props.hit.fragments || []).filter(f => f.role === 'body' && f.identity?.document_key === props.hit.doc && !!prefix && f.identity?.anchor?.startsWith(prefix) && citationFromAnchor(f.identity?.anchor))
  return labels.map((label, i) => ({ label, anchor: fragments[i]?.identity?.anchor }))
})
const source = computed(() => {
  try { const url = new URL(doc.value?.source_url || ''); return url.protocol === 'https:' ? url.href : null } catch { return null }
})
function memberLink(memberAnchor: string | undefined) {
  return memberAnchor && props.release ? readerLink({ ...props.hit, handle: `${props.hit.doc}#${memberAnchor}` }, props.release) : null
}
</script>

<template>
  <article class="card result-card">
    <div class="result-card-head">
      <ValidityStamp :status="doc?.status.class" />
      <NuxtLink v-if="doc?.number && link" :to="link" class="num result-number">{{ doc.number }}</NuxtLink><span v-else-if="doc?.number" class="num">{{ doc.number }}</span>
      <span v-if="doc" class="result-byline">{{ hitByline(doc, language) }}</span>
      <span v-if="rank" class="result-rank">#{{ rank }}</span>
    </div>
    <div class="result-card-body">
      <p v-if="doc?.title" class="result-title">{{ doc.title }}</p>
      <h2>{{ hit.citation }}<template v-if="heading"> · {{ heading }}</template></h2>
      <p v-if="members.length > 1" class="result-members">{{ messages.matched.replace('{n}', String(members.length)) }} <template v-for="(member, index) in members" :key="index"><NuxtLink v-if="memberLink(member.anchor)" :to="memberLink(member.anchor)!"><template v-if="index">, </template>{{ member.label }}</NuxtLink><span v-else><template v-if="index">, </template>{{ member.label }}</span></template></p>
      <p v-if="text" class="law-text result-text" :class="{ 'result-text--clamped': !expanded }"><template v-for="(piece, index) in pieces" :key="index"><mark v-if="piece.marked" :aria-label="`${messages.matchedWords}: ${piece.text}`">{{ piece.text }}</mark><template v-else>{{ piece.text }}</template></template></p>
      <button v-if="text" type="button" class="result-toggle" :aria-expanded="expanded" @click="expanded = !expanded">{{ expanded ? messages.collapse : messages.expand }}</button>
      <p v-for="(warning, index) in hit.warnings" :key="index" class="notice result-warning"><template v-if="warning.number && release && warningLink(warning, release)"><NuxtLink :to="warningLink(warning, release)!">{{ warning.number }}</NuxtLink>{{ warningSentence(warning, language).slice(warning.number.length) }}</template><template v-else>{{ warningSentence(warning, language) }}</template> {{ warningAdvice(warning.kind, language) }}</p>
      <p v-if="hit.warnings_omitted" class="notice result-warning"><NuxtLink v-if="link" :to="link">{{ messages.warningMore.replace('{n}', String(hit.warnings_omitted)) }}</NuxtLink><template v-else>{{ messages.warningMore.replace('{n}', String(hit.warnings_omitted)) }}</template></p>
      <ul v-if="hit.gaps?.length" class="result-gaps"><li v-for="(gap, index) in hit.gaps" :key="index">{{ gapCategory(gap.kind, language).name }}</li></ul>
    </div>
    <footer class="result-card-footer">
      <NuxtLink v-if="link" :to="link">{{ messages.read }}</NuxtLink>
      <CopyButton v-if="citation" :text="citation" :label="messages.copy" />
      <a v-if="source" :href="source" target="_blank" rel="noopener noreferrer">vbpl.vn ↗</a>
      <div class="result-nodes"><template v-for="code in hit.nodes" :key="code"><NuxtLink v-if="nodes?.[code]?.name" class="chip" :to="`/vn/topics/${code}`">{{ nodes[code]?.name }}</NuxtLink></template></div>
    </footer>
  </article>
</template>
