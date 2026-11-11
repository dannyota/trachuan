<script setup lang="ts">
import { getCoverage } from '../api/coverage'
import rawSample from '../data/sample-result.json'
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, statusMessage, type Language } from '../../i18n'
const sample = rawSample as components['schemas']['SearchResult']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi()
const { data: status } = await useAsyncData('coverage-home', () => import.meta.server ? getCoverage(() => api.client.GET('/api/v1/public/status')).then(status => ({ status })) : Promise.resolve({ status: null }), { server: true, default: () => ({ status: null }) })
</script>
<template>
  <div>
    <h1>{{ messages.home }}</h1><p>{{ messages.purpose }}</p>
    <a href="/api/v1/auth/google/start?next=%2F">{{ messages.signIn }}</a>
    <NuxtLink to="/connect">{{ messages.connect }}</NuxtLink>
    <CoverageFacts :status="status.status" />
    <section><h2>{{ messages.sample }}</h2>
      <article v-for="hit in (sample.hits || []).slice(0, 1)" :key="hit.handle || hit.citation">
        <p>{{ hit.citation }}</p><p>{{ statusMessage(language, hit.doc ? sample.documents?.[hit.doc]?.status.class : undefined) }}</p><p>{{ hit.text || hit.snippet }}</p>
      </article>
    </section>
  </div>
</template>
