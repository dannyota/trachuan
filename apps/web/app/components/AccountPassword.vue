<script setup lang="ts">
import ReauthForm from './ReauthForm.vue'
import ReauthNotice from './ReauthNotice.vue'
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
      if (reauth.requireProof(result.error, action, 'password-link', session.account?.password === true, nextCode => change(action, nextCode))) return
      error.value = authError(language.value, result.error.error.code, result.response.status)
      return
    }
    notice.value = changing ? labels.value.success : labels.value.addPasswordSent
    clear()
    if (changing) emit('changed', signal)
    await api.loadSession(signal)
  } catch { if (!signal.aborted) error.value = labels.value.generic }
  finally { busy.value = false }
}
</script>
<template>
  <section id="reauth-password-link" class="account-row account-security-row" :aria-label="`${labels.password}: ${session.account?.password ? labels.passwordSet : labels.passwordUnset}`">
    <div class="account-security-head">
      <h3>{{ labels.password }}<span class="account-security-status">{{ session.account?.password ? labels.passwordSet : labels.passwordUnset }}</span></h3>
      <button v-if="session.account?.password && !editing && !reauth.notice.value" class="btn btn-secondary" type="button" @click="editing = true">{{ labels.changePassword }}</button>
      <button v-else-if="!session.account?.password && !reauth.notice.value" class="btn btn-secondary" type="button" :disabled="busy" @click="change()">{{ labels.addPassword }}</button>
    </div>
    <p v-if="error" class="account-security-notice" role="alert">{{ error }}</p><p v-if="notice" class="account-security-notice" role="status" aria-live="polite">{{ notice }}</p>
    <ReauthNotice v-if="reauth.notice.value" :action="reauth.notice.value" @sign-in="reauth.signInAgain" @cancel="clear" />
    <ReauthForm v-else-if="reauth.challenge.value" v-bind="reauth.challenge.value" @verified="reauth.verified" @cancel="clear" />
    <form v-else-if="session.account?.password && editing" class="auth-form" @submit.prevent="change()">
      <label for="username">{{ labels.email }}<input id="username" :value="session.account.email" class="input" type="email" name="username" autocomplete="username" readonly></label>
      <label for="current-password">{{ labels.currentPassword }}<input id="current-password" v-model="current" class="input" type="password" name="current-password" autocomplete="current-password" required></label>
      <label for="new-password">{{ labels.newPassword }}<input id="new-password" v-model="password" class="input" type="password" name="new-password" autocomplete="new-password" minlength="15" passwordrules="minlength: 15; maxlength: 128;" aria-describedby="change-password-help" required></label>
      <p id="change-password-help">{{ labels.passwordHelp }}</p>
      <label v-if="session.account.totp">{{ labels.code }}<input v-model="code" class="input" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}"></label>
      <button class="btn btn-primary" type="submit" :disabled="busy">{{ labels.savePassword }}</button>
    </form>
  </section>
</template>
