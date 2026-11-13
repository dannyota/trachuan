<script setup lang="ts">
import { getCoverage } from '../api/coverage'
import rawSample from '../data/sample-result.json'
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
import { homeMessages } from '../../i18n/home'
const sample = rawSample as components['schemas']['SearchResult']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => homeMessages(language.value))
const route = useRoute()
const deleted = ref(false)
onMounted(() => { deleted.value = route.query.account === 'deleted' })
const api = useApi()
const { data: status } = await useAsyncData('coverage-home', () => import.meta.server ? getCoverage(() => api.client.GET('/api/v1/public/status')).then(status => ({ status })) : Promise.resolve({ status: null }), { server: true, default: () => ({ status: null }) })
</script>
<template>
  <div>
    <p v-if="deleted" class="container notice" role="status">{{ messages.deletedNotice }}</p>
    <HomeHero />
    <section class="container home-sample" aria-labelledby="home-sample-title">
      <div><h2 id="home-sample-title">{{ labels.sampleTitle }}</h2><p>{{ labels.sampleText }}</p><ul><li v-for="item in [labels.check1, labels.check2, labels.check3]" :key="item">{{ item }}</li></ul></div>
      <ResultCard v-if="sample.hits?.[0]" :hit="sample.hits[0]" :documents="sample.documents" :nodes="sample.nodes" current-release />
    </section>
    <div class="container home-coverage"><CoverageFacts :status="status.status" :show-release="false" /><p>{{ labels.scope }} <NuxtLink to="/sources">{{ labels.scopeLink }}</NuxtLink></p></div>
    <HomeConnect />
  </div>
</template>
