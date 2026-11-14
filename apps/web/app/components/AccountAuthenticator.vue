<script setup lang="ts">
import AuthenticatorQr from './AuthenticatorQr.vue'
import RecoveryCodes from './RecoveryCodes.vue'
import ReauthForm from './ReauthForm.vue'
import { useReauth } from '../composables/useReauth'
import { usePageSecrets } from '../composables/usePageSecrets'
import { computed, ref, watch } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import { authError, authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import { useSessionStore } from '../stores/session'
import type { Action } from '../api/client'

const emit = defineEmits<{ changed: [signal: AbortSignal] }>()
const session = useSessionStore()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const api = useApi({ userAction: true, localUnavailable: true })
const reauth = useReauth()
const setup = ref<components['schemas']['TOTPSetup'] | null>(null)
const setupAction = ref<Action | null>(null)
const codes = ref<string[]>([])
const code = ref('')
const recovery = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const enrollmentOff = ref(false)
function clear() { setup.value = null; setupAction.value = null; codes.value = []; code.value = ''; reauth.cancel() }
const secrets = usePageSecrets(clear)
watch(() => session.account?.id, clear)
type Operation = 'setup' | 'confirm' | 'remove' | 'recovery'
async function mutate(kind: Operation, action = kind === 'confirm' ? setupAction.value || api.captureAction() : api.captureAction(), proof?: string) {
  const signal = secrets.signal
  if (busy.value || signal.aborted) return
  busy.value = true; error.value = ''; notice.value = ''; codes.value = []
  const client = api.forAction(action)
  const params = { header: { 'X-CSRF-Token': '' } }
  try {
    const result = kind === 'setup'
      ? await client.POST('/api/v1/me/totp/setup', { params, signal })
      : kind === 'confirm'
        ? await client.POST('/api/v1/me/totp/confirm', { params, body: { code: code.value }, signal })
        : kind === 'remove'
          ? await client.DELETE('/api/v1/me/totp', { params, body: proof ? { code: proof } : recovery.value ? { recovery_code: code.value } : { code: code.value }, signal })
          : await client.POST('/api/v1/me/recovery-codes', { params, body: { code: proof || code.value }, signal })
    if (signal.aborted) return
    if (result.error) {
      if (reauth.requireProof(result.error, action, kind === 'remove' || kind === 'recovery', nextCode => mutate(kind, action, nextCode))) return
      if (result.response.status === 404 && (kind === 'setup' || kind === 'confirm')) { enrollmentOff.value = true; setup.value = null; error.value = labels.value.enrollmentOff }
      else error.value = authError(language.value, result.error.error.code, result.response.status)
      return
    }
    if (kind === 'setup' && result.data && 'otpauth_uri' in result.data && typeof result.data.otpauth_uri === 'string' && 'secret' in result.data && typeof result.data.secret === 'string') { setup.value = { otpauth_uri: result.data.otpauth_uri, secret: result.data.secret }; setupAction.value = action; return }
    setup.value = null; setupAction.value = null; code.value = ''
    if (result.data && 'recovery_codes' in result.data) codes.value = result.data.recovery_codes
    notice.value = labels.value.success
    emit('changed', signal)
    await api.loadSession(signal)
  } catch { if (!signal.aborted) error.value = labels.value.generic }
  finally { busy.value = false }
}
function toggle() { recovery.value = !recovery.value; code.value = '' }
</script>
<template>
  <section aria-labelledby="account-authenticator-title">
    <h3 id="account-authenticator-title">{{ labels.factorTitle }}</h3>
    <p v-if="error" class="notice" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <RecoveryCodes v-if="codes.length" :codes="codes" @close="codes = []" />
    <ReauthForm v-if="reauth.challenge.value" v-bind="reauth.challenge.value" @verified="reauth.verified" @cancel="clear" />
    <template v-else-if="setup">
      <AuthenticatorQr :uri="setup.otpauth_uri" :label="labels.qr" />
      <p>{{ labels.secret }}</p><p class="auth-secret">{{ setup.secret }}</p>
      <form class="auth-form" @submit.prevent="mutate('confirm')">
        <label>{{ labels.code }}<input v-model="code" class="input" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" required></label>
        <div class="auth-actions"><button class="btn btn-primary" :disabled="busy">{{ labels.confirm }}</button><button class="btn btn-secondary" type="button" @click="clear">{{ labels.cancel }}</button></div>
      </form>
    </template>
    <template v-else>
      <button v-if="session.account?.totp_enrollment && !enrollmentOff" class="btn btn-secondary" type="button" :disabled="busy" @click="mutate('setup')">{{ labels.setup }}</button>
      <form v-if="session.account?.totp" class="auth-form" @submit.prevent="mutate('remove')">
        <label>{{ recovery ? labels.recovery : labels.code }}<input v-model="code" class="input" :inputmode="recovery ? 'text' : 'numeric'" :autocomplete="recovery ? 'off' : 'one-time-code'" :pattern="recovery ? undefined : '[0-9]{6}'" required></label>
        <div class="auth-actions"><button class="btn btn-secondary" type="button" @click="toggle">{{ recovery ? labels.useCode : labels.useRecovery }}</button><button class="btn btn-secondary" :disabled="busy">{{ labels.remove }}</button><button v-if="!recovery" class="btn btn-secondary" type="button" :disabled="busy || !code" @click="mutate('recovery')">{{ labels.replaceCodes }}</button></div>
      </form>
    </template>
  </section>
</template>
