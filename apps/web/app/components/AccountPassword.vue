<script setup lang="ts">
import ReauthForm from './ReauthForm.vue'
import { useReauth } from '../composables/useReauth'
import { usePageSecrets } from '../composables/usePageSecrets'
import { computed, ref, watch } from 'vue'
import { authError, authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import { useSessionStore } from '../stores/session'

const emit = defineEmits<{ changed: [signal: AbortSignal] }>()
const session = useSessionStore()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const api = useApi({ userAction: true, localUnavailable: true })
const reauth = useReauth()
const current = ref('')
const password = ref('')
const code = ref('')
const editing = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
function clear() { current.value = ''; password.value = ''; code.value = ''; reauth.cancel() }
const secrets = usePageSecrets(clear)
watch(() => session.account?.id, clear)
async function change(action = api.captureAction(), proof?: string) {
  const signal = secrets.signal
  if (busy.value || signal.aborted) return
  busy.value = true; error.value = ''; notice.value = ''
  const changing = session.account?.password === true
  const client = api.forAction(action)
  try {
    const result = changing
      ? await client.POST('/api/v1/me/password', { params: { header: { 'X-CSRF-Token': '' } }, body: { current: current.value, password: password.value, ...((proof || code.value) ? { code: proof || code.value } : {}) }, signal })
      : await client.POST('/api/v1/me/password/link', { params: { header: { 'X-CSRF-Token': '' } }, signal })
    if (signal.aborted) return
    if (result.error) {
      if (reauth.requireProof(result.error, action, session.account?.password === true, nextCode => change(action, nextCode))) return
      error.value = authError(language.value, result.error.error.code, result.response.status)
      return
    }
    notice.value = changing ? labels.value.success : labels.value.mail
    clear()
    if (changing) emit('changed', signal)
    await api.loadSession(signal)
  } catch { if (!signal.aborted) error.value = labels.value.generic }
  finally { busy.value = false }
}
</script>
<template>
  <section class="account-row" aria-labelledby="account-password-title">
    <h3 id="account-password-title">{{ labels.password }}: {{ session.account?.password ? labels.passwordSet : labels.passwordUnset }}</h3>
    <button v-if="session.account?.password && !editing" class="btn btn-secondary" type="button" @click="editing = true">{{ labels.changePassword }}</button>
    <p v-if="error" class="notice" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <ReauthForm v-if="reauth.challenge.value" v-bind="reauth.challenge.value" @verified="reauth.verified" @cancel="clear" />
    <form v-else-if="session.account?.password && editing" class="auth-form" @submit.prevent="change()">
      <label>{{ labels.currentPassword }}<input v-model="current" class="input" type="password" autocomplete="current-password" required></label>
      <label>{{ labels.newPassword }}<input v-model="password" class="input" type="password" autocomplete="new-password" aria-describedby="change-password-help" required></label>
      <p id="change-password-help">{{ labels.passwordHelp }}</p>
      <label v-if="session.account.totp">{{ labels.code }}<input v-model="code" class="input" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}"></label>
      <button class="btn btn-primary" :disabled="busy">{{ labels.savePassword }}</button>
    </form>
    <button v-else-if="!session.account?.password" class="btn btn-secondary" type="button" :disabled="busy" @click="change()">{{ labels.addPassword }}</button>
  </section>
</template>
