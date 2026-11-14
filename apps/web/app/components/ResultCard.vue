<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, watch } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import { documentCitation, gapCategory, highlightSpans, hitByline, matchedMembers, statusAsOf, warningAdvice, warningSentence } from '../content'
import { displayHeading } from '../utils/reader-units'
import { readerLink, warningLink } from '../utils/search-results'
import { searchMessages } from '../../i18n/search'
import type { Language } from '../../i18n'
import ValidityStamp from './ValidityStamp.vue'
import CopyButton from './CopyButton.vue'

type Hit = components['schemas']['Hit']
type Result = components['schemas']['SearchResult']
const props = defineProps<{ hit: Hit, documents?: Result['documents'], nodes?: Result['nodes'], release?: string, currentRelease?: boolean, statusDate?: string, query?: string, rank?: number }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => searchMessages(language.value))
const doc = computed(() => props.documents?.[props.hit.doc || ''])
const linkRelease = computed(() => props.currentRelease ? undefined : props.release)
const canLink = computed(() => props.currentRelease || !!props.release)
const link = computed(() => canLink.value ? readerLink(props.hit, linkRelease.value) : null)
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
  const seen = new Set<string>()
  return (props.hit.fragments || []).flatMap((fragment) => {
    const memberAnchor = fragment.identity?.anchor
    const label = matchedMembers({ ...props.hit, fragments: [fragment] })[0]
    if (!memberAnchor || !label || seen.has(memberAnchor)) return []
    seen.add(memberAnchor)
    return [{ label, anchor: memberAnchor }]
  })
})
const textElement = ref<HTMLElement | null>(null)
const canExpand = ref(false)
let observer: ResizeObserver | undefined
let mounted = false
function measureText() {
  const element = textElement.value
  if (mounted && element && !expanded.value) canExpand.value = element.scrollHeight > element.clientHeight
}
watch(text, async () => { expanded.value = false; await nextTick(); measureText() })
watch(expanded, async (value) => { if (!value) { await nextTick(); measureText() } })
onMounted(() => {
  mounted = true
  measureText()
  if (typeof ResizeObserver !== 'undefined' && textElement.value) {
    observer = new ResizeObserver(measureText)
    observer.observe(textElement.value)
  }
  document.fonts?.ready.then(measureText)
})
onUnmounted(() => { mounted = false; observer?.disconnect() })
const source = computed(() => {
  try { const url = new URL(doc.value?.source_url || ''); return url.protocol === 'https:' ? url.href : null } catch { return null }
})
function memberLink(memberAnchor: string | undefined) {
  return memberAnchor && canLink.value ? readerLink({ ...props.hit, handle: `${props.hit.doc}#${memberAnchor}` }, linkRelease.value) : null
}
</script>

<template>
  <article class="card result-card">
    <div class="result-card-head">
      <ValidityStamp :status="doc?.status.class" /><span v-if="statusDate" class="result-byline">{{ statusAsOf(statusDate, doc?.source_url, language) }}</span>
      <NuxtLink v-if="doc?.number && link" :to="link" class="num result-number">{{ doc.number }}</NuxtLink><span v-else-if="doc?.number" class="num">{{ doc.number }}</span>
      <span v-if="doc" class="result-byline">{{ hitByline(doc, language) }}</span>
      <span v-if="rank" class="result-rank">#{{ rank }}</span>
    </div>
    <div class="result-card-body">
      <p v-if="doc?.title" class="result-title">{{ doc.title }}</p>
      <h2>{{ displayHeading(hit.citation, heading) }}</h2>
      <p v-if="members.length > 1" class="result-members">{{ messages.matched.replace('{n}', String(members.length)) }} <template v-for="(member, index) in members" :key="index"><NuxtLink v-if="memberLink(member.anchor)" :to="memberLink(member.anchor)!"><template v-if="index">, </template>{{ member.label }}</NuxtLink><span v-else><template v-if="index">, </template>{{ member.label }}</span></template></p>
      <p v-if="text" ref="textElement" class="law-text result-text" :class="{ 'result-text--clamped': !expanded }"><template v-for="(piece, index) in pieces" :key="index"><mark v-if="piece.marked" :aria-label="`${messages.matchedWords}: ${piece.text}`">{{ piece.text }}</mark><template v-else>{{ piece.text }}</template></template></p>
      <button v-if="canExpand" type="button" class="result-toggle" :aria-expanded="expanded" @click="expanded = !expanded">{{ expanded ? messages.collapse : messages.expand }}</button>
      <p v-for="(warning, index) in hit.warnings" :key="index" class="notice result-warning"><template v-if="warning.number && canLink && warningLink(warning, linkRelease)"><NuxtLink :to="warningLink(warning, linkRelease)!">{{ warning.number }}</NuxtLink>{{ warningSentence(warning, language).slice(warning.number.length) }}</template><template v-else>{{ warningSentence(warning, language) }}</template> {{ warningAdvice(warning.kind, language) }}</p>
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
