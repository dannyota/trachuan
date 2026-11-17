<script setup lang="ts">
import { frameworkMessages } from '../../../i18n/frameworks'
import { versionPath } from '../../utils/framework-handles'
import { errorMessage } from '../../../i18n'
import FrameworkAccess from '../../components/FrameworkAccess.vue'

definePageMeta({ signedIn: true })
const { api, language, notFound } = useFrameworkPage()
const labels = computed(() => frameworkMessages(language.value))
const { data, clear } = await useAsyncData('framework-list', async () => {
  const response = await api.client.GET('/api/v1/frameworks')
  if (response.response.status === 404) notFound()
  return { result: response.data, error: response.error?.error.code }
})
onBeforeUnmount(clear)
</script>
<template>
  <main class="container page-content fw-page">
    <div class="fw-list-header"><h1>{{ labels.frameworks }}</h1><NuxtLink class="btn btn-secondary" to="/frameworks/search">{{ labels.search }}</NuxtLink></div>
    <p v-if="data?.error" role="alert">{{ errorMessage(language, data.error) }}</p>
    <template v-if="data?.result">
      <p v-if="data.result.access_unavailable" class="notice">{{ labels.accessUnavailable }}</p>
      <p class="fw-meta fw-summary">{{ labels.counts(data.result.totals.controls, data.result.totals.edges_resolved, data.result.totals.edges_unresolved) }}</p>
      <section v-for="(entry, handle) in data.result.versions" :key="handle" class="fw-row">
        <h2 class="fw-list-identifier"><NuxtLink :to="versionPath(handle)" class="fw-mono">{{ handle.slice(3).replace(':', ' ') }}</NuxtLink></h2>
        <p class="fw-meta"><FrameworkAccess :access="entry.access" /> · {{ entry.readable ? labels.readable : labels.unreadable }}</p>
        <p class="fw-meta">{{ labels.counts(entry.controls, entry.edges_resolved, entry.edges_unresolved) }}</p>
        <p v-for="(target, key) in entry.lineage" :key="key" class="fw-meta"><NuxtLink :to="target ? versionPath(target) : undefined" class="fw-mono">{{ target }}</NuxtLink></p>
        <p v-for="source in entry.sources" :key="source.name" class="fw-meta">{{ source.name }} · {{ source.text_source }} · {{ source.text_complete ? labels.complete : labels.incomplete }}</p>
      </section>
    </template>
  </main>
</template>
<style scoped>
.fw-mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.fw-list-identifier { margin: 0; font-size: 16px; }
</style>
