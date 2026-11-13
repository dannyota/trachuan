<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import ReportsSection from '../components/ReportsSection.vue'
import { accountActions } from '../api/account-actions'
import type { Action } from '../api/client'
import { useSessionStore } from '../stores/session'
import { errorMessage, messagesFor, type Language } from '../../i18n'
import { accountMessages } from '../../i18n/account'
import { formatDateTime } from '../content'

definePageMeta({ signedIn: true })
type Session = components['schemas']['Session']
type Token = components['schemas']['Token']
type Grant = components['schemas']['Grant']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => accountMessages(language.value))
const session = useSessionStore()
const api = useApi({ userAction: true })
const actions = accountActions(api.client)
const sessions = ref<Session[]>([])
const tokens = ref<Token[]>([])
const grants = ref<Grant[]>([])
const error = ref('')
const creating = ref(false)
const deleting = ref(false)
const tokenAction = ref<Action | null>(null)
const deletionAction = ref<Action | null>(null)
function openToken() {
  tokenAction.value = api.captureAction()
  creating.value = true
}
const deletionEmail = ref('')
function openDeletion() {
  if (!session.account) return
  deletionAction.value = api.captureAction()
  deletionEmail.value = session.account.email
  deleting.value = true
}
const busy = ref(false)
const loading = ref(true)
const orderedSessions = computed(() => [...sessions.value].sort((a, b) => Number(b.current) - Number(a.current)))
const currentSession = computed(() => sessions.value.find(item => item.current))
function creationDate(value: string) { return formatDateTime(value, language.value).replace(/,? \d{2}:\d{2}$/, '') }
function date(value: string | null) { return value ? formatDateTime(value, language.value) : messages.value.none }
function showError(code?: string) { error.value = errorMessage(language.value, code || 'generic') }
async function loadSessions() {
  const result = await api.client.GET('/api/v1/me/sessions')
  if (result.data) sessions.value = result.data
  else showError(result.error?.error.code)
}
async function loadTokens() {
  const result = await api.client.GET('/api/v1/me/tokens')
  if (result.data) tokens.value = result.data
  else showError(result.error?.error.code)
}
async function loadGrants() {
  const result = await api.client.GET('/api/v1/me/grants')
  if (result.data) grants.value = result.data
  else showError(result.error?.error.code)
}
onMounted(async () => {
  try { await Promise.all([loadSessions(), loadTokens(), loadGrants()]) }
  catch { showError() }
  finally { loading.value = false }
})
async function revokeSession(item: Session) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    const result = await actions.revokeSession(item)
    if (result.error) showError(result.error.error.code)
    else if (item.current) { session.$reset(); session.status = 'signed-out'; await navigateTo('/') }
    else await loadSessions()
  } catch { showError() } finally { busy.value = false }
}
async function revokeToken(id: number) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { const result = await actions.revokeToken(id); if (result.error) showError(result.error.error.code); else await loadTokens() }
  catch { showError() } finally { busy.value = false }
}
async function revokeGrant(id: number) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { const result = await actions.revokeGrant(id); if (result.error) showError(result.error.error.code); else await loadGrants() }
  catch { showError() } finally { busy.value = false }
}
async function tokenCreated() { try { await loadTokens() } catch { showError() } }
</script>
<template>
  <div class="account-page">
    <div class="account-heading">
      <div><h1>{{ labels.title }}</h1><p v-if="session.account"><span>{{ session.account.email }}</span> · {{ labels.created }} {{ creationDate(session.account.created_at) }}</p></div>
      <button class="btn btn-secondary" type="button" :disabled="busy || !currentSession" @click="currentSession && revokeSession(currentSession)">{{ labels.signOut }}</button>
    </div>
    <p v-if="error" class="notice" role="alert" aria-live="assertive">{{ error }}</p>
    <section class="card account-card"><div class="account-card-head"><div><h2>{{ labels.sessions }}</h2><p>{{ labels.sessionsDescription }}</p></div></div>
      <div v-if="loading" class="account-skeleton" role="status"><span class="visually-hidden">{{ labels.loading }}</span></div>
      <p v-else-if="!orderedSessions.length && !error" class="account-empty">{{ labels.sessionsEmpty }}</p>
      <article v-for="item in orderedSessions" :key="item.id" class="account-row">
        <div class="account-row-label"><strong>{{ item.current ? labels.thisBrowser : labels.otherSession }}</strong><span v-if="item.current" class="account-current">{{ labels.current }}</span></div>
        <p class="account-meta">{{ labels.signedIn }} {{ date(item.created_at) }} · {{ labels.lastSeen }} {{ date(item.last_seen_at) }}</p>
        <button class="btn btn-secondary" type="button" :disabled="busy" @click="revokeSession(item)">{{ item.current ? labels.signOut : labels.revoke }}</button>
      </article>
    </section>
    <section class="card account-card"><div class="account-card-head"><div><h2>{{ labels.tokens }}</h2><p>{{ labels.tokensDescription }}</p></div><button class="btn btn-primary" type="button" :disabled="busy" @click="openToken">{{ labels.createToken }}</button></div>
      <div v-if="loading" class="account-skeleton" role="status"><span class="visually-hidden">{{ labels.loading }}</span></div>
      <p v-else-if="!tokens.length && !error" class="account-empty">{{ labels.tokensEmpty }}</p>
      <article v-for="item in tokens" :key="item.id" class="account-row"><div class="account-row-label"><h3>{{ item.name }}</h3><span v-if="item.revoked_at" class="account-revoked">{{ labels.revoked }}</span></div>
        <p class="account-meta">{{ labels.created }} {{ date(item.created_at) }} · {{ labels.expires }} {{ date(item.expires_at) }} · {{ labels.lastUsed }} {{ date(item.last_used_at) }}<template v-if="item.revoked_at"> · {{ labels.revoked }} {{ date(item.revoked_at) }}</template></p>
        <button v-if="!item.revoked_at" class="btn btn-secondary" type="button" :disabled="busy" @click="revokeToken(item.id)">{{ labels.revoke }}</button>
      </article>
    </section>
    <section class="card account-card"><div class="account-card-head"><div><h2>{{ labels.grants }}</h2><p>{{ labels.grantsDescription }}</p></div></div>
      <div v-if="loading" class="account-skeleton" role="status"><span class="visually-hidden">{{ labels.loading }}</span></div>
      <p v-else-if="!grants.length && !error" class="account-empty">{{ labels.grantsEmpty }}</p>
      <article v-for="item in grants" :key="item.id" class="account-row"><div class="account-row-label"><h3>{{ item.client_name || item.client_id_host || messages.none }}</h3><span v-if="item.client_name && item.client_id_host" class="account-host">{{ item.client_id_host }}</span></div>
        <p class="account-meta">{{ labels.granted }} {{ date(item.created_at) }} · {{ labels.lastUsed }} {{ date(item.last_used_at) }}</p><button class="btn btn-secondary" type="button" :disabled="busy" @click="revokeGrant(item.id)">{{ labels.revokeGrant }}</button>
      </article>
    </section>
    <ReportsSection />
    <section class="card account-danger"><div><h2>{{ labels.deleteAccount }}</h2><p>{{ labels.deleteDescription }}</p></div><button class="btn btn-danger" type="button" :disabled="busy" @click="openDeletion">{{ labels.deleteButton }}</button></section>
    <TokenCreateDialog v-if="creating && tokenAction" :action="tokenAction" @close="creating = false" @created="tokenCreated" />
    <AccountDeleteDialog v-if="deleting && deletionAction" :email="deletionEmail" :action="deletionAction" @close="deleting = false" />
  </div>
</template>
