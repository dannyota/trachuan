<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { accountActions } from '../api/account-actions'
import { useSessionStore } from '../stores/session'
import { errorMessage, messagesFor, type Language } from '../../i18n'

definePageMeta({ signedIn: true })
type Session = components['schemas']['Session']
type Token = components['schemas']['Token']
type Grant = components['schemas']['Grant']
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const session = useSessionStore()
const api = useApi({ userAction: true })
const actions = accountActions(api.client)
const sessions = ref<Session[]>([])
const tokens = ref<Token[]>([])
const grants = ref<Grant[]>([])
const error = ref('')
const creating = ref(false)
const deleting = ref(false)
const deletionEmail = ref('')
function openDeletion() {
  if (!session.account) return
  deletionEmail.value = session.account.email
  deleting.value = true
}
const busy = ref(false)
function date(value: string | null) { return value ? new Intl.DateTimeFormat(language.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : messages.value.none }
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
  <div>
    <h1>{{ messages.account }}</h1>
    <div v-if="session.account"><p>{{ session.account.email }}</p><p>{{ messages.accountCreated }}: {{ date(session.account.created_at) }}</p></div>
    <p v-if="error" role="alert">{{ error }}</p>
    <section><h2>{{ messages.sessions }}</h2>
      <article v-for="item in sessions" :key="item.id" class="account-row">
        <p>{{ messages.accountCreated }}: {{ date(item.created_at) }}</p><p>{{ messages.lastSeen }}: {{ date(item.last_seen_at) }}</p>
        <p v-if="item.current">{{ messages.currentSession }}</p>
        <button type="button" :disabled="busy" @click="revokeSession(item)">{{ item.current ? messages.signOut : messages.revoke }}</button>
      </article>
    </section>
    <section><h2>{{ messages.personalTokens }}</h2><button type="button" @click="creating = true">{{ messages.createToken }}</button>
      <article v-for="item in tokens" :key="item.id" class="account-row">
        <h3>{{ item.name }}</h3><p>{{ messages.accountCreated }}: {{ date(item.created_at) }}</p><p>{{ messages.expires }}: {{ date(item.expires_at) }}</p>
        <p>{{ messages.lastUsed }}: {{ date(item.last_used_at) }}</p><p>{{ messages.revoked }}: {{ date(item.revoked_at) }}</p>
        <button v-if="!item.revoked_at" type="button" :disabled="busy" @click="revokeToken(item.id)">{{ messages.revoke }}</button>
      </article>
    </section>
    <section><h2>{{ messages.clients }}</h2>
      <article v-for="item in grants" :key="item.id" class="account-row">
        <h3>{{ item.client_name || item.client_id_host || item.id }}</h3><p v-if="item.client_name && item.client_id_host">{{ item.client_id_host }}</p>
        <p>{{ messages.accountCreated }}: {{ date(item.created_at) }}</p><p>{{ messages.lastUsed }}: {{ date(item.last_used_at) }}</p>
        <button type="button" :disabled="busy" @click="revokeGrant(item.id)">{{ messages.revoke }}</button>
      </article>
    </section>
    <section><h2>{{ messages.deleteAccount }}</h2><button type="button" @click="openDeletion">{{ messages.deleteAccount }}</button></section>
    <TokenCreateDialog v-if="creating" @close="creating = false" @created="tokenCreated" />
    <AccountDeleteDialog v-if="deleting" :email="deletionEmail" @close="deleting = false" />
  </div>
</template>
