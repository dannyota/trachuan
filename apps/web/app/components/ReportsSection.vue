<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { errorMessage, messagesFor, statusMessage, type Language } from '../../i18n'
import { isDocumentKey } from '../utils/document-key'

type Report = components['schemas']['Report']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi({ userAction: true })
const reports = ref<Report[]>([])
const next = ref<string | undefined>()
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
function date(value: string) { return new Intl.DateTimeFormat(language.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
function kind(value: string) {
  const labels: Record<string, string> = { wrong_text: messages.value.reportWrongText, wrong_validity: messages.value.reportWrongValidity, broken_citation: messages.value.reportBrokenCitation, missing_document: messages.value.reportMissingDocument }
  return Object.hasOwn(labels, value) ? labels[value]! : value
}
function state(value: string) {
  const labels: Record<string, string> = { open: messages.value.reportOpen, accepted: messages.value.reportAccepted, rejected: messages.value.reportRejected, withdrawn: messages.value.reportWithdrawn }
  return Object.hasOwn(labels, value) ? labels[value]! : value
}
function link(item: Report) {
  return isDocumentKey(item.document_key) ? `/vn/documents/${item.document_key}${item.anchor ? `#${encodeURIComponent(item.anchor)}` : ''}` : ''
}
async function load(cursor?: string) {
  busy.value = true
  try {
    const result = await api.client.GET('/api/v1/me/reports', { params: { query: cursor ? { next: cursor } : {} } })
    if (result.data) {
      reports.value = cursor ? [...reports.value, ...result.data.reports] : result.data.reports
      next.value = result.data.next
      loaded.value = true
    } else error.value = errorMessage(language.value, result.error?.error.code || 'generic')
  } catch { error.value = errorMessage(language.value, 'generic') }
  finally { busy.value = false }
}
async function more() { if (!busy.value && next.value) { error.value = ''; await load(next.value) } }
async function withdraw(item: Report) {
  if (busy.value) return
  const action = api.captureAction()
  busy.value = true
  error.value = ''
  try {
    const result = await api.forAction(action).DELETE('/api/v1/me/reports/{id}', { params: { path: { id: item.report_id }, header: { 'X-CSRF-Token': '' } } })
    if (result.error?.error.code === 'not_found') { error.value = messages.value.reportDecided; await load() }
    else if (result.error) error.value = errorMessage(language.value, result.error.error.code)
    else await load()
  } catch { error.value = errorMessage(language.value, 'generic') }
  finally { busy.value = false }
}
onMounted(() => load())
</script>
<template>
  <section>
    <h2>{{ messages.reports }}</h2>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="loaded && !reports.length">{{ messages.reportsEmpty }}</p>
    <article v-for="item in reports" :key="item.report_id" class="account-row">
      <p>{{ messages.reportKind }}: {{ kind(item.kind) }}</p>
      <p>{{ messages.reportDocument }}: <a v-if="link(item)" :href="link(item)" class="document-number">{{ item.document_number || item.document_key }}</a><span v-else class="document-number">{{ item.document_number || item.document_key || messages.none }}</span></p>
      <p>{{ messages.reportAnchor }}: {{ item.anchor || messages.none }}</p>
      <p>{{ messages.reportTarget }}: {{ item.target || messages.none }}</p>
      <p>{{ messages.reportState }}: <span>{{ state(item.state) }}</span></p>
      <p>{{ messages.accountCreated }}: {{ date(item.created_at) }}</p>
      <p>{{ messages.reportChanged }}: {{ date(item.changed_at) }}</p>
      <div><p>{{ messages.reportSuggestion }}:</p>
        <p v-if="item.suggestion.status_class">{{ messages.reportStatus }}: {{ statusMessage(language, item.suggestion.status_class) }}</p>
        <p v-if="item.suggestion.eff_from">{{ messages.readerEffectiveFrom }}: {{ item.suggestion.eff_from }}</p>
        <p v-if="item.suggestion.eff_to">{{ messages.readerEffectiveTo }}: {{ item.suggestion.eff_to }}</p>
        <p v-if="item.suggestion.document_number">{{ messages.reportDocument }}: {{ item.suggestion.document_number }}</p>
        <p v-if="item.suggestion.issue_date">{{ messages.reportIssueDate }}: {{ item.suggestion.issue_date }}</p>
        <p v-if="!Object.keys(item.suggestion).length">{{ messages.none }}</p>
      </div>
      <button v-if="item.state === 'open'" type="button" :disabled="busy" @click="withdraw(item)">{{ messages.reportWithdraw }}</button>
    </article>
    <button v-if="next" class="reports-more" type="button" :disabled="busy" @click="more">{{ messages.reportsMore }}</button>
  </section>
</template>
