<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { frameworkMessages } from '../../i18n/frameworks'
import { errorMessage } from '../../i18n'
import { frameworkItemError } from '../utils/framework-read'

type Unit = components['schemas']['FrameworkReadUnit']
const props = defineProps<{ handle: string, kind: 'version' | 'control' }>()
const { api, language, notFound } = useFrameworkPage()
const labels = computed(() => frameworkMessages(language.value))
const units = ref<Unit[]>([])
const next = ref('')
const release = ref('')
const notice = ref('')
const error = ref('')
const unavailable = ref(false)
const loading = ref(false)
async function load(more = false) {
  if (loading.value) return
  loading.value = true
  const query = more ? { next: next.value, release: release.value } : {}
  const params = { path: { handle: props.handle }, query }
  const response = props.kind === 'version'
    ? await api.client.GET('/api/v1/frameworks/versions/{handle}', { params })
    : await api.client.GET('/api/v1/frameworks/controls/{handle}', { params })
  loading.value = false
  if (response.response.status === 404 || frameworkItemError(response.error?.error.code) === 'not-found') { notFound(); return }
  if (!response.data) { error.value = response.error?.error.code || 'unavailable'; return }
  const item = response.data.items?.[0]
  const itemError = frameworkItemError(item?.error)
  if (itemError === 'not-found') { notFound(); return }
  if (itemError === 'restart' && more) { notice.value = labels.value.reload; units.value = []; next.value = ''; await load(); return }
  if (itemError) error.value = itemError
  release.value = response.data.release
  unavailable.value = response.data.access_unavailable === true
  units.value = more ? [...units.value, ...(item?.units || [])] : item?.units || []
  next.value = item?.next || ''
}
await load()
</script>
<template>
  <div>
    <FrameworkUnits :units="units" :handle="handle"><template #notices><p v-if="unavailable" class="notice">{{ labels.accessUnavailable }}</p><p v-if="notice" role="status">{{ notice }}</p></template></FrameworkUnits>
    <p v-if="error" class="container notice" role="alert">{{ error === 'unit_too_large' || error === 'invalid_item' ? labels[error] : errorMessage(language, error) }}</p>
    <button v-if="next && !error" class="container btn btn-secondary" :disabled="loading" type="button" @click="load(true)">{{ labels.next }}</button>
  </div>
</template>
