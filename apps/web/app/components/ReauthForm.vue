<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import type { Action } from '../api/client'
import { authError, authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'

const props = defineProps<{ action: Action, factor: boolean }>()
const emit = defineEmits<{ verified: [code?: string], cancel: [] }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const api = useApi({ userAction: true, localUnavailable: true })
const client = api.forAction(props.action)
const password = ref('')
const code = ref('')
const busy = ref(false)
const error = ref('')
const controller = new AbortController()
onBeforeUnmount(() => { controller.abort(); password.value = ''; code.value = '' })
async function verify() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    const result = await client.POST('/api/v1/me/reauth', { params: { header: { 'X-CSRF-Token': '' } }, body: { password: password.value }, signal: controller.signal })
    if (controller.signal.aborted) return
    if (result.error) error.value = authError(language.value, result.error.error.code, result.response.status)
    else emit('verified', code.value || undefined)
  } catch { if (!controller.signal.aborted) error.value = labels.value.generic }
  finally { password.value = ''; busy.value = false }
}
</script>
<template>
  <form class="auth-form" @submit.prevent="verify">
    <p>{{ labels.reauth }}</p>
    <label>{{ labels.password }}<input v-model="password" class="input" type="password" autocomplete="current-password" required></label>
    <label v-if="factor">{{ labels.code }}<input v-model="code" class="input" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" required></label>
    <p v-if="error" class="notice" role="alert">{{ error }}</p>
    <div class="auth-actions"><button class="btn btn-primary" :disabled="busy">{{ labels.verify }}</button><button class="btn btn-secondary" type="button" @click="emit('cancel')">{{ labels.cancel }}</button></div>
  </form>
</template>
