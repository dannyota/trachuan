<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import ReaderRail from '../../../components/ReaderRail.vue'
import { joinedUnits, warningsByTarget } from '../../../utils/reader-units'
import ReaderHeader from '../../../components/ReaderHeader.vue'
import ReaderToc from '../../../components/ReaderToc.vue'
import ReaderUnit from '../../../components/ReaderUnit.vue'
import { readerMessages } from '../../../../i18n/reader'
import { errorMessage, messagesFor, type Language } from '../../../../i18n'

definePageMeta({ signedIn: true, layout: 'reader' })
type Unit = components['schemas']['Unit']
type Document = components['schemas']['Document']
type Gap = components['schemas']['Gap']
const route = useRoute()
const router = useRouter()
const api = useApi({ localUnavailable: true })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const readerMessagesText = computed(() => readerMessages(language.value))
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
const visibleAnchor = ref('')
let observer: IntersectionObserver | undefined
let unitObserver: IntersectionObserver | undefined
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
function observeUnits() {
  unitObserver?.disconnect()
  const top = Math.floor(window.innerHeight * 0.1)
  const bottom = Math.floor(window.innerHeight * 0.75)
  unitObserver = new IntersectionObserver(entries => {
    const visible = entries.find(entry => entry.isIntersecting)
    if (visible) visibleAnchor.value = (visible.target as HTMLElement).dataset.anchor || ''
  }, { rootMargin: `-${top}px 0px -${bottom}px 0px` })
  content.value?.querySelectorAll('[data-anchor]').forEach(element => unitObserver?.observe(element))
}
onMounted(async () => {
  mounted = true
  const initialHash = route.hash
  if (typeof IntersectionObserver !== 'undefined') {
    observer = new IntersectionObserver(entries => { if (entries.some(entry => entry.isIntersecting)) void fillVisible(generation, controller.signal) }, { rootMargin: '600px' })
    observeUnits()
    window.addEventListener('resize', observeUnits)
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
  unitObserver?.disconnect()
  window.removeEventListener('resize', observeUnits)
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
watch(textUnits, async () => {
  await nextTick()
  unitObserver?.disconnect()
  content.value?.querySelectorAll('[data-anchor]').forEach(element => unitObserver?.observe(element))
})
</script>

<template>
  <div>
    <ReaderHeader v-if="documentEntry" :document="documentEntry" :release="release" />
    <div v-else-if="loading && !pages" class="reader-identity reader-skeleton" role="status"><div class="container"><span class="visually-hidden">{{ readerMessagesText.loading }}</span><span class="reader-skeleton-line reader-skeleton-short" /><span class="reader-skeleton-line reader-skeleton-title" /><span class="reader-skeleton-line" /></div></div>
    <div class="container reader">
      <div class="reader-toc-desktop"><h2>{{ readerMessagesText.contents }}</h2><ReaderToc :units="outline" :current="route.hash.slice(1) || visibleAnchor" /></div>
      <section ref="content" class="reader-content" aria-live="polite">
        <details class="reader-toc-mobile"><summary>{{ readerMessagesText.contents }}</summary><ReaderToc :units="outline" :current="route.hash.slice(1) || visibleAnchor" /></details>
        <div v-if="loading && !pages" class="reader-skeleton" aria-hidden="true"><span class="reader-skeleton-line reader-skeleton-short" /><span class="reader-skeleton-line" /><span class="reader-skeleton-line" /><span class="reader-skeleton-line" /></div>
        <ReaderUnit v-for="(unit, index) in textUnits" :key="index" :unit="unit" :document="documentEntry" :release="release" :target="!!unit.anchor && unit.anchor === route.hash.slice(1)" :warnings="warnings.get(`${key}#${unit.anchor}`) || []" />
        <p v-if="error" class="notice" role="alert">{{ errorMessage(language, error) }} <a v-if="error === 'release_expired'" :href="`/vn/documents/${key}`">{{ messages.readerCurrent }}</a></p>
        <div v-if="next && !error" class="reader-more"><button ref="sentinel" class="btn btn-secondary" type="button" :disabled="loading" @click="load">{{ messages.readerMore }}</button><span v-if="loading" role="status"><span class="reader-spinner" aria-hidden="true" /> {{ readerMessagesText.loading }}</span></div>
      </section>
      <ReaderRail v-if="release" :document="documentEntry" :nodes="nodes" :gaps="gaps" :release="release" :hash="hash" :units="textUnits" :document-key="key" :warnings="warnings" />
    </div>
  </div>
</template>
