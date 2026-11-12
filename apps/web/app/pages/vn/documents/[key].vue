<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import ReaderRail from '../../../components/ReaderRail.vue'
import { joinedUnits, warningsByTarget } from '../../../utils/reader-units'
import { warningLink, warningTarget, warningVerb } from '../../../utils/search-results'
import { errorMessage, messagesFor, type Language } from '../../../../i18n'

definePageMeta({ signedIn: true })
type Unit = components['schemas']['Unit']
type Document = components['schemas']['Document']
type Gap = components['schemas']['Gap']
const route = useRoute()
const router = useRouter()
const api = useApi({ localUnavailable: true })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const key = computed(() => String(route.params.key))
const release = ref('')
const hash = ref('')
const documentEntry = ref<Document>()
const gaps = ref<Gap[]>([])
const nodes = ref<components['schemas']['NodeIndex']>({})
const rawUnits = ref<Unit[]>([])
const units = computed(() => joinedUnits(rawUnits.value))
const outline = computed(() => units.value.filter(unit => unit.kind === 'outline'))
const textUnits = computed(() => units.value.filter(unit => unit.kind !== 'outline'))
const warnings = computed(() => warningsByTarget(rawUnits.value))
const next = ref('')
const error = ref('')
const loading = ref(false)
const pages = ref(0)
const sentinel = ref<HTMLElement>()
const content = ref<HTMLElement>()
let observer: IntersectionObserver | undefined
let seeking = false
let generation = 0
let pending: Promise<void> | undefined
let controller = new AbortController()
let disposed = false
let mounted = false
let filling = false

function active(version: number, signal: AbortSignal) {
  const current = router.currentRoute.value
  return !disposed && version === generation && !signal.aborted && current.path === `/vn/documents/${key.value}` && JSON.stringify(current.query.release) === JSON.stringify(route.query.release)
}

function scrollReader(version: number, signal: AbortSignal, target?: HTMLElement) {
  const current = router.currentRoute.value
  if (!mounted || !active(version, signal) || current.hash !== route.hash) return
  if (target) target.scrollIntoView()
  else window.scrollTo(0, 0)
}

function requestedRelease() {
  const value = route.query.release
  return typeof value === 'string' && /^[0-9]{8}(?:_[0-9]+)?$/.test(value) ? value : undefined
}
function load(): Promise<void> {
  if (disposed || controller.signal.aborted) return Promise.resolve()
  if (pending) return pending
  if (error.value || (pages.value > 0 && !next.value)) return Promise.resolve()
  const version = generation
  const signal = controller.signal
  const request = requestPage(version, signal)
  pending = request
  void request.finally(() => {
    if (!active(version, signal)) return
    if (pending === request) pending = undefined
    if (!seeking) void fillVisible(version, signal)
  })
  return request
}
async function requestPage(version: number, signal: AbortSignal) {
  loading.value = true
  const continuation = next.value
  try {
    const response = await api.client.GET('/api/v1/documents/{key}', { params: { path: { key: key.value }, query: { ...(continuation ? { next: continuation, release: release.value } : requestedRelease() ? { release: requestedRelease() } : {}) } }, signal })
    if (!active(version, signal)) return
    if (!response.data) {
      error.value = response.error?.error?.code || 'unavailable'
      return
    }
    const page = response.data
    if (page.item.error) { error.value = page.item.error; return }
    if (!pages.value) {
      release.value = page.release
      hash.value = page.fetch_sha256 || ''
      documentEntry.value = page.item.document
    }
    Object.assign(nodes.value, page.nodes || {})
    rawUnits.value.push(...(page.item.units || []))
    gaps.value.push(...(page.item.gaps || []))
    next.value = page.item.next || ''
    pages.value++
  } catch {
    if (active(version, signal)) error.value = 'unavailable'
  } finally {
    if (active(version, signal)) loading.value = false
  }
}
async function fillVisible(version: number, signal: AbortSignal) {
  if (!observer || filling || seeking || !active(version, signal)) return
  filling = true
  try {
    await nextTick()
    while (active(version, signal) && !seeking && !error.value && next.value && pages.value < 50 && sentinel.value) {
      const rect = sentinel.value.getBoundingClientRect()
      if (rect.top > window.innerHeight + 600 || rect.bottom < -600) break
      await load()
      await nextTick()
    }
  } finally {
    if (active(version, signal)) filling = false
  }
}
function findAnchor(anchor: string) {
  return Array.from(content.value?.querySelectorAll<HTMLElement>('[data-anchor]') || []).find(element => element.dataset.anchor === anchor)
}
async function seekAnchor(initial = false) {
  if (disposed || controller.signal.aborted || seeking) return
  seeking = true
  const anchor = route.hash.slice(1)
  const seekGeneration = generation
  const signal = controller.signal
  if (!anchor) {
    if (initial) scrollReader(seekGeneration, signal)
    seeking = false
    void fillVisible(seekGeneration, signal)
    return
  }
  await nextTick()
  if (!active(seekGeneration, signal)) return
  let target = findAnchor(anchor)
  while (!target && next.value && !error.value && pages.value < 50 && active(seekGeneration, signal)) {
    await load()
    await nextTick()
    if (!active(seekGeneration, signal)) return
    target = findAnchor(anchor)
  }
  if (!active(seekGeneration, signal)) return
  if (route.hash.slice(1) !== anchor) {
    seeking = false
    await seekAnchor()
    return
  }
  if (target) scrollReader(seekGeneration, signal, target)
  else scrollReader(seekGeneration, signal)
  seeking = false
  void fillVisible(seekGeneration, signal)
}
onMounted(async () => {
  mounted = true
  const initialHash = route.hash
  if (typeof IntersectionObserver !== 'undefined') {
    observer = new IntersectionObserver(entries => { if (entries.some(entry => entry.isIntersecting)) void fillVisible(generation, controller.signal) }, { rootMargin: '600px' })
  }
  const version = generation
  const signal = controller.signal
  await load()
  if (active(version, signal)) await seekAnchor(route.hash === initialHash)
})
onBeforeUnmount(() => {
  mounted = false
  disposed = true
  generation++
  controller.abort()
  observer?.disconnect()
})
watch(sentinel, (element, old) => {
  if (old) observer?.unobserve(old)
  if (element) observer?.observe(element)
})
watch([key, () => route.query.release], async () => {
  controller.abort()
  generation++
  controller = new AbortController()
  pending = undefined
  filling = false
  release.value = ''
  hash.value = ''
  documentEntry.value = undefined
  gaps.value = []
  nodes.value = {}
  rawUnits.value = []
  next.value = ''
  error.value = ''
  loading.value = false
  pages.value = 0
  seeking = false
  const version = generation
  const signal = controller.signal
  const initialHash = route.hash
  await load()
  if (active(version, signal)) await seekAnchor(route.hash === initialHash)
})
watch(() => route.hash, () => { void seekAnchor() })
</script>

<template>
  <div class="reader">
    <nav :aria-label="messages.readerContents">
      <h2 v-if="outline.length">{{ messages.readerContents }}</h2>
      <ul><li v-for="(unit, index) in outline" :key="index"><a v-if="unit.anchor" :href="`#${encodeURIComponent(unit.anchor)}`">{{ unit.label }} {{ unit.heading }}</a><span v-else>{{ unit.label }} {{ unit.heading }}</span></li></ul>
    </nav>
    <section ref="content" class="reader-content">
      <article v-for="(unit, index) in textUnits" :key="index" :data-anchor="unit.anchor">
        <h2 v-if="unit.label || unit.heading">{{ unit.label }} {{ unit.heading }}</h2>
        <div v-if="(warnings.get(`${key}#${unit.anchor}`)?.length || unit.warnings_omitted)" class="warnings"><p v-for="(warning, i) in warnings.get(`${key}#${unit.anchor}`)" :key="i"><NuxtLink v-if="warningLink(warning, release)" :to="warningLink(warning, release)!">{{ warning.number || warning.key }}</NuxtLink><span v-else>{{ warning.number || warning.key }}</span> {{ warningVerb(warning.kind, messages) }} <code v-if="warningTarget(warning).raw">{{ warningTarget(warning).text }}</code><template v-else>{{ warningTarget(warning).text }}</template><template v-if="!warning.exact"> {{ ' ' + messages.warningMissing }}</template><template v-if="warning.basis === 'vbpl_reference'"> {{ ' ' + messages.warningReference }}</template>.</p><p v-if="unit.warnings_omitted">{{ messages.warningMore.replace('{n}', String(unit.warnings_omitted)) }}</p></div>
        <p class="law-text">{{ unit.text }}</p>
      </article>
      <p v-if="error" role="alert">{{ errorMessage(language, error) }} <a v-if="error === 'release_expired'" :href="`/vn/documents/${key}`">{{ messages.readerCurrent }}</a></p>
      <button v-if="next && !error" ref="sentinel" type="button" :disabled="loading" @click="load">{{ messages.readerMore }}</button>
    </section>
    <ReaderRail v-if="release" :document="documentEntry" :nodes="nodes" :gaps="gaps" :release="release" :hash="hash" :units="textUnits" :document-key="key" :warnings="warnings" />
  </div>
</template>
