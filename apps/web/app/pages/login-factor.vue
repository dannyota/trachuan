<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { authError, authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import { safeNext } from '../api/next'

definePageMeta({ path: '/login/factor' })

const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const client = useAuthApi()
const csrf = ref('')
const code = ref('')
const recovery = ref(false)
const busy = ref(false)
const error = ref('')
const secrets = usePageSecrets(() => { csrf.value = ''; code.value = '' }, loadPending)
async function pending(signal: AbortSignal) {
  const result = await client.GET('/api/v1/auth/factor', { signal })
  if (signal.aborted) return false
  if (result.data) { csrf.value = result.data.csrf; return true }
  if (result.response.status === 401) window.location.replace('/login')
  else error.value = authError(language.value, result.error?.error.code, result.response.status)
  return false
}
async function loadPending() {
  const signal = secrets.signal
  try { await pending(signal) } catch { if (!signal.aborted) error.value = labels.value.generic }
}
onMounted(loadPending)
function toggle() { recovery.value = !recovery.value; code.value = ''; error.value = '' }
async function verify() {
  const signal = secrets.signal
  if (busy.value || !csrf.value || signal.aborted) return
  busy.value = true; error.value = ''
  try {
    const result = await client.POST('/api/v1/auth/factor', { params: { header: { Origin: window.location.origin, 'X-CSRF-Token': csrf.value } }, body: recovery.value ? { recovery_code: code.value } : { code: code.value }, signal })
    if (signal.aborted) return
    if (result.data) window.location.assign(safeNext(result.data.next))
    else {
      error.value = authError(language.value, result.error?.error.code, result.response.status)
      if (result.response.status === 401 || result.response.status === 403) await pending(signal)
    }
  } catch { if (!signal.aborted) error.value = labels.value.generic }
  finally { busy.value = false; code.value = '' }
}
</script>
<template>
  <div class="container login-page"><section class="card login-card">
    <h1>{{ labels.factorTitle }}</h1>
    <form class="auth-form" @submit.prevent="verify">
      <label>{{ recovery ? labels.recovery : labels.code }}<input v-model="code" class="input" :inputmode="recovery ? 'text' : 'numeric'" :autocomplete="recovery ? 'off' : 'one-time-code'" :pattern="recovery ? undefined : '[0-9]{6}'" required></label>
      <p v-if="error" class="notice" role="alert">{{ error }}</p>
      <button class="btn btn-primary" :disabled="busy || !csrf">{{ labels.verify }}</button>
      <button class="btn btn-secondary" type="button" @click="toggle">{{ recovery ? labels.useCode : labels.useRecovery }}</button>
    </form>
  </section></div>
</template>
