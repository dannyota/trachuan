<script setup lang="ts">
import { getCoverage } from '../api/coverage'
import rawSample from '../data/sample-result.json'
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
import { homeMessages } from '../../i18n/home'
import { documentCitation } from '../content'
const sample = rawSample as components['schemas']['SearchResult']
const hit = sample.hits?.[0]
const doc = hit?.doc ? sample.documents?.[hit.doc] : undefined
const citation = doc ? documentCitation(hit?.handle?.split('#', 2)[1], doc) : null
let hasSource = false
try { hasSource = new URL(doc?.source_url || '').protocol === 'https:' } catch { hasSource = false }
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => homeMessages(language.value))
const route = useRoute()
const deleted = ref(false)
onMounted(() => { deleted.value = route.query.account === 'deleted' })
const api = useApi()
const releaseDate = /^(\d{4})(\d{2})(\d{2})(?:_\d+)?$/.exec(sample.release || '')
const statusDate = releaseDate ? `${releaseDate[1]}-${releaseDate[2]}-${releaseDate[3]}` : undefined
const { data: status } = await useAsyncData('coverage-home', () => import.meta.server ? getCoverage(() => api.client.GET('/api/v1/public/status')).then(status => ({ status })) : Promise.resolve({ status: null }), { server: true, default: () => ({ status: null }) })
</script>
<template>
  <div class="home-page">
    <p v-if="deleted" class="container notice" role="status">{{ messages.deletedNotice }}</p>
    <HomeHero />
    <section class="container home-sample" aria-labelledby="home-sample-title">
      <div><h2 id="home-sample-title">{{ labels.sampleTitle }}</h2><p>{{ labels.sampleText }}</p><ul><li>{{ labels.check1 }}</li><li v-if="citation">{{ labels.check2 }}: {{ citation }}</li><li v-if="hasSource">{{ labels.check3 }}</li></ul></div>
      <ResultCard v-if="sample.hits?.[0]" :hit="sample.hits[0]" :documents="sample.documents" :nodes="sample.nodes" :status-date="statusDate" current-release />
    </section>
    <div class="container home-coverage"><CoverageFacts :status="status.status" :show-release="false" /><p>{{ labels.scope }} <NuxtLink to="/sources">{{ labels.scopeLink }}</NuxtLink></p></div>
    <HomeConnect />
  </div>
</template>
