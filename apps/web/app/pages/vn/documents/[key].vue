<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import ReaderRail from '../../../components/ReaderRail.vue'
import { joinedUnits } from '../../../utils/reader-units'
import { errorMessage, messagesFor, type Language } from '../../../../i18n'

definePageMeta({ signedIn: true })
type Unit = components['schemas']['Unit']
type Document = components['schemas']['Document']
type Gap = components['schemas']['Gap']
const route = useRoute()
const api = useApi({ localUnavailable: true })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const key = computed(() => String(route.params.key))
const release = ref('')
const hash = ref('')
const documentEntry = ref<Document>()
const gaps = ref<Gap[]>([])
const rawUnits = ref<Unit[]>([])
const units = computed(() => joinedUnits(rawUnits.value))
const outline = computed(() => units.value.filter(unit => unit.kind === 'outline'))
const textUnits = computed(() => units.value.filter(unit => unit.kind !== 'outline'))
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

function requestedRelease() {
  const value = route.query.release
  return typeof value === 'string' && /^[0-9]{8}_[0-9]+$/.test(value) ? value : undefined
}
function load(): Promise<void> {
  if (pending) return pending
  if (error.value || (pages.value > 0 && !next.value)) return Promise.resolve()
  const request = requestPage()
  pending = request
  void request.finally(() => { if (pending === request) pending = undefined })
  return request
}
async function requestPage() {
  loading.value = true
  const continuation = next.value
  const requestGeneration = generation
  try {
    const response = await api.client.GET('/api/v1/documents/{key}', { params: { path: { key: key.value }, query: { ...(continuation ? { next: continuation, release: release.value } : requestedRelease() ? { release: requestedRelease() } : {}) } } })
    if (requestGeneration !== generation) return
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
    rawUnits.value.push(...(page.item.units || []))
    gaps.value.push(...(page.item.gaps || []))
    next.value = page.item.next || ''
    pages.value++
  } catch {
    if (requestGeneration === generation) error.value = 'unavailable'
  } finally {
    if (requestGeneration === generation) loading.value = false
  }
}
function findAnchor(anchor: string) {
  return Array.from(content.value?.querySelectorAll<HTMLElement>('[data-anchor]') || []).find(element => element.dataset.anchor === anchor)
}
async function seekAnchor() {
  if (seeking) return
  seeking = true
  const anchor = route.hash.slice(1)
  const seekGeneration = generation
  if (!anchor) { window.scrollTo(0, 0); seeking = false; return }
  await nextTick()
  let target = findAnchor(anchor)
  while (!target && next.value && !error.value && pages.value < 50 && seekGeneration === generation) {
    await load()
    await nextTick()
    target = findAnchor(anchor)
  }
  if (seekGeneration !== generation) return
  if (route.hash.slice(1) !== anchor) {
    seeking = false
    await seekAnchor()
    return
  }
  if (target) target.scrollIntoView()
  else window.scrollTo(0, 0)
  seeking = false
  if (sentinel.value && observer) {
    observer.unobserve(sentinel.value)
    observer.observe(sentinel.value)
  }
}
onMounted(async () => {
  if (typeof IntersectionObserver !== 'undefined') {
    observer = new IntersectionObserver(entries => { if (entries.some(entry => entry.isIntersecting) && !seeking) void load() }, { rootMargin: '600px' })
  }
  await load()
  await seekAnchor()
})
onBeforeUnmount(() => observer?.disconnect())
watch(sentinel, (element, old) => {
  if (old) observer?.unobserve(old)
  if (element) observer?.observe(element)
})
watch([key, requestedRelease], async () => {
  generation++
  pending = undefined
  release.value = ''
  hash.value = ''
  documentEntry.value = undefined
  gaps.value = []
  rawUnits.value = []
  next.value = ''
  error.value = ''
  loading.value = false
  pages.value = 0
  seeking = false
  await load()
  await seekAnchor()
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
        <p class="law-text">{{ unit.text }}</p>
      </article>
      <p v-if="error" role="alert">{{ errorMessage(language, error) }} <a v-if="error === 'release_expired'" :href="`/vn/documents/${key}`">{{ messages.readerCurrent }}</a></p>
      <button v-if="next && !error" ref="sentinel" type="button" :disabled="loading" @click="load">{{ messages.readerMore }}</button>
    </section>
    <ReaderRail v-if="release" :document="documentEntry" :gaps="gaps" :release="release" :hash="hash" />
  </div>
</template>
