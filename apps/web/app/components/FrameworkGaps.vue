<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { frameworkMessages } from '../../i18n/frameworks'
import { errorMessage } from '../../i18n'
const props = defineProps<{ handle: string }>()
const { api, language, notFound } = useFrameworkPage()
const labels = computed(() => frameworkMessages(language.value))
const gaps = ref<components['schemas']['FrameworkGap'][]>([])
const cursor = ref('')
const release = ref('')
const error = ref('')
const notice = ref('')
const unavailable = ref(false)
const loading = ref(false)
async function load(more = false, restart: false | 'release' | 'cursor' = false) {
  if (loading.value) return
  loading.value = true
  error.value = ''
  const response = await api.client.GET('/api/v1/frameworks/gaps', { params: { query: { versions: [props.handle], ...(more ? { cursor: cursor.value, release: release.value } : {}) } } })
  loading.value = false
  if (response.response.status === 404) { notFound(); return }
  if (!response.data) {
    const code = response.error?.error.code || 'unavailable'
    if (more && code === 'cursor_expired') { notice.value = ''; gaps.value = []; cursor.value = ''; await load(false, 'cursor'); return }
    if (more && code === 'release_expired') { notice.value = ''; await load(false, 'release'); return }
    if (restart === 'release' && code === 'invalid_request') { notFound(); return }
    error.value = code
    return
  }
  notice.value = restart ? labels.value.restarted : ''
  unavailable.value = response.data.access_unavailable === true
  release.value = response.data.release
  gaps.value = more ? [...gaps.value, ...(response.data.gaps || [])] : response.data.gaps || []
  cursor.value = response.data.next_cursor || ''
}
function gapLabel(kind: string) { return labels.value.gap[kind as keyof typeof labels.value.gap] || kind }
await load()
</script>
<template>
  <section class="container fw-content">
    <h2 class="fw-section-title">{{ labels.gaps }}</h2>
    <p v-if="unavailable" class="notice">{{ labels.accessUnavailable }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <div v-for="(gap, index) in gaps" :key="index" class="fw-row"><span v-if="gap.subject" class="num">{{ gap.subject }}</span><p class="fw-title">{{ gapLabel(gap.kind) }}</p><p class="fw-meta">{{ gap.detail }}</p></div>
    <p v-if="error" role="alert">{{ errorMessage(language, error) }}</p><button v-if="cursor" type="button" class="btn btn-secondary" :disabled="loading" @click="load(true)">{{ labels.more }}</button>
  </section>
</template>
