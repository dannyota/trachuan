<script setup lang="ts">
import { getCoverage } from '../api/coverage'
import rawSample from '../data/sample-result.json'
import sampleMeta from '../data/sample-result.meta.json'
import { loginUrl } from '../api/next'
import { useSessionStore } from '../stores/session'
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
import { homeMessages } from '../../i18n/home'
import { documentCitation, formatDate } from '../content'
useHead({ htmlAttrs: { 'data-page': 'home' } })
const session = useSessionStore()
const mounted = ref(false)
onMounted(() => { mounted.value = true })
const searchLink = computed(() => mounted.value && session.status === 'signed-in' ? '/vn/search' : loginUrl('/vn/search'))
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
const sampleDate = `${sampleMeta.release.slice(0, 4)}-${sampleMeta.release.slice(4, 6)}-${sampleMeta.release.slice(6, 8)}`
const { data: status } = await useAsyncData('coverage-home', () => import.meta.server ? getCoverage(() => api.client.GET('/api/v1/public/status')).then(status => ({ status })) : Promise.resolve({ status: null }), { server: true, default: () => ({ status: null }) })
</script>
<template>
  <div class="home-page">
    <p v-if="deleted" class="container notice" role="status">{{ messages.deletedNotice }}</p>
    <HomeHero />
    <div class="container home-sample"><span v-if="citation" class="home-citation">{{ citation }}</span><ResultCard v-if="sample.hits?.[0]" :hit="sample.hits[0]" :documents="sample.documents" :nodes="sample.nodes" :status-date="statusDate" current-release /></div>
    <section class="home-explanation" aria-labelledby="home-sample-title"><div class="container">
      <h2 id="home-sample-title">{{ labels.sampleTitle.replace('{date}', formatDate(sampleDate, language)) }}</h2><p>{{ labels.sampleText }}</p><ul><li>{{ labels.check1 }}</li><li v-if="citation">{{ labels.check2.replace('{citation}', citation) }}</li><li v-if="hasSource">{{ labels.check3 }}</li></ul>
    </div></section>
    <div class="container home-coverage"><CoverageFacts :status="status.status" /><p>{{ labels.scope }} <NuxtLink to="/sources">{{ labels.scopeLink }}</NuxtLink></p></div>
    <HomeConnect />
    <section class="container home-maker"><div class="home-maker-panel"><img src="/maker-avatar-320.webp" srcset="/maker-avatar-320.webp 320w, /maker-avatar-640.webp 640w" sizes="(min-width: 64rem) 232px, (min-width: 48rem) 176px, 168px" :alt="labels.makerAlt"><div><h2>{{ labels.makerTitle }}</h2><p>{{ labels.makerText }}</p><div class="home-maker-actions"><NuxtLink class="btn btn-primary" :to="searchLink">{{ labels.makerSearch }}</NuxtLink><a class="btn home-outline" href="https://aboutme.vn/danny">{{ labels.makerResume }}</a><a class="btn home-outline" href="mailto:support@trachuan.vn">{{ labels.makerFeedback }}</a></div></div></div></section>
  </div>
</template>
