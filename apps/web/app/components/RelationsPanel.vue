<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import { ApiError, type Action } from '../api/client'
import { legalBases, relationGroups } from '../content'
import { isDocumentKey } from '../utils/document-key'
import { joinedUnits } from '../utils/reader-units'
import { errorMessage, type Language } from '../../i18n'
import { readerMessages } from '../../i18n/reader'

type Unit = components['schemas']['Unit']
type Relation = components['schemas']['ListedRelation']
const props = defineProps<{ document: components['schemas']['Document'], documentKey: string, release: string }>()
const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
const api = useApi({ localUnavailable: true })
const controller = new AbortController()
const raw = ref<Unit[]>([])
const loaded = ref(false)
const loading = ref(false)
const next = ref('')
const error = ref('')
let action: Action | undefined
const relations = computed(() => joinedUnits(raw.value).flatMap(unit => unit.relation ? [unit.relation] : []))
const bases = computed(() => legalBases(relations.value))
const groups = computed(() => loaded.value ? relationGroups(relations.value.filter(item => !bases.value.includes(item)), undefined, language.value) : relationGroups(props.document.related, props.document.related_counts, language.value))
function path(item: Relation | components['schemas']['Pointer']) {
  return item.in_release && item.key && isDocumentKey(item.key) ? { path: `/vn/documents/${item.key}`, query: { release: props.release } } : undefined
}
async function load() {
  if (loading.value || controller.signal.aborted || (loaded.value && !next.value)) return
  action ||= api.captureAction()
  loading.value = true
  error.value = ''
  try {
    const response = await api.forAction(action).POST('/api/v1/read', { body: { country: 'vn', release: props.release, items: [next.value ? { next: next.value } : { handle: props.documentKey, relations: true }] }, signal: controller.signal, params: { header: { 'X-CSRF-Token': action.csrfToken || '' } } })
    if (controller.signal.aborted) return
    if (api.session.account?.id !== action.accountId) { error.value = 'account_changed'; return }
    if (!response.data) { error.value = response.error?.error?.code || 'unavailable'; return }
    const item = response.data.items?.[0]
    if (!item || item.error) { error.value = item?.error || 'unavailable'; return }
    raw.value.push(...(item.units || []))
    next.value = item.next || ''
    loaded.value = true
  } catch (cause) {
    if (!controller.signal.aborted) error.value = cause instanceof ApiError ? cause.code : 'unavailable'
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}
function expand(event: Event) { if ((event.target as HTMLDetailsElement).open && !loaded.value && !error.value) void load() }
onBeforeUnmount(() => controller.abort())
</script>
<template>
  <section class="card reader-card">
    <h2>{{ m.relations }}</h2>
    <dl class="reader-relations">
      <template v-for="group in groups" :key="group.label">
        <dt>{{ group.label }}</dt><dd><template v-for="(item, index) in group.items" :key="index"><NuxtLink v-if="path(item)" class="reader-relation-chip num" :to="path(item)!">{{ item.number || item.key }}</NuxtLink><span v-else class="reader-relation-chip num">{{ item.number || item.key }}</span></template><button v-if="group.more" class="reader-relation-chip" type="button" :aria-label="`${m.moreRelations}: ${group.label}`" @click="load">+{{ group.more }}</button></dd>
      </template>
      <template v-if="bases.length"><dt>{{ m.legalBases }}</dt><dd><template v-for="(item, index) in bases" :key="index"><NuxtLink v-if="path(item)" class="reader-relation-chip num" :to="path(item)!">{{ item.number || item.key }}</NuxtLink><span v-else class="reader-relation-chip num">{{ item.number || item.key }}</span></template></dd></template>
    </dl>
    <details @toggle="expand"><summary>{{ m.moreRelations }}</summary><p v-if="loading" role="status">{{ m.loading }}</p><button v-if="next && !loading" class="btn btn-secondary" type="button" @click="load">{{ m.moreRelations }}</button></details>
    <p v-if="error" class="notice" role="alert">{{ errorMessage(language, error) }} <button type="button" @click="load">{{ m.retry }}</button></p>
  </section>
</template>
