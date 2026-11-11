<script setup lang="ts">
import { onBeforeRouteLeave } from 'vue-router'
import { accountActions } from '../api/account-actions'
import { errorMessage, messagesFor, type Language } from '../../i18n'

const emit = defineEmits<{ close: [], created: [] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const dialog = ref<HTMLDialogElement | null>(null)
onMounted(() => {
  dialog.value?.showModal()
  window.addEventListener('pagehide', close)
  window.addEventListener('pageshow', restored)
})
const api = useApi({ userAction: true })
const name = ref('')
const days = ref(90)
const token = ref('')
const error = ref('')
const busy = ref(false)
let active = true
function clear() { active = false; token.value = '' }
function close() { clear(); emit('close') }
function restored(event: PageTransitionEvent) { if (event.persisted) close() }
onBeforeRouteLeave(clear)
onBeforeUnmount(() => {
  clear()
  window.removeEventListener('pagehide', close)
  window.removeEventListener('pageshow', restored)
})
async function create() {
  if (busy.value || !name.value || !Number.isInteger(days.value) || days.value < 1 || days.value > 365) return
  busy.value = true
  error.value = ''
  try {
    const result = await accountActions(api.client).createToken(name.value, days.value)
    if (!active) return
    if (result.data) { token.value = result.data.token; emit('created') }
    else error.value = errorMessage(language.value, result.error?.error.code || 'generic')
  } catch { if (active) error.value = errorMessage(language.value, 'generic') }
  finally { busy.value = false }
}
async function copy() {
  try { await navigator.clipboard.writeText(token.value) } catch { error.value = errorMessage(language.value, 'generic') }
}
</script>
<template>
  <dialog ref="dialog" @cancel.prevent="close">
    <h2>{{ messages.createToken }}</h2>
    <form v-if="!token" @submit.prevent="create">
      <label>{{ messages.name }} <input v-model="name" required maxlength="64"></label>
      <label>{{ messages.days }} <input v-model.number="days" type="number" min="1" max="365" required></label>
      <button type="submit" :disabled="busy">{{ messages.create }}</button>
    </form>
    <div v-else><p>{{ messages.tokenOnce }}</p><p class="token-value">{{ token }}</p><button type="button" @click="copy">{{ messages.copy }}</button></div>
    <p v-if="error" role="alert">{{ error }}</p>
    <button type="button" @click="close">{{ messages.close }}</button>
  </dialog>
</template>
