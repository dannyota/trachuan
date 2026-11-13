<script setup lang="ts">
import { messagesFor, errorMessage, type Language } from '../../i18n'
import { accountActions } from '../api/account-actions'
import type { Action } from '../api/client'
import { useSessionStore } from '../stores/session'
import { wrapDialogFocus } from '../utils/dialog-focus'

const props = defineProps<{ email: string, action: Action }>()
const emit = defineEmits<{ close: [] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const dialog = ref<HTMLDialogElement | null>(null)
let opener: HTMLElement | null = null
onMounted(() => {
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  dialog.value?.showModal()
})
function close() {
  dialog.value?.close()
  if (opener?.isConnected) opener.focus()
  emit('close')
}
const confirmation = ref('')
const error = ref('')
const busy = ref(false)
const accountChanged = ref(false)
const api = useApi({ userAction: true })
const actions = accountActions(api.forAction(props.action))
const session = useSessionStore()
async function remove() {
  if (confirmation.value !== props.email || busy.value || accountChanged.value) return
  busy.value = true
  try {
    const result = await actions.deleteAccount()
    if (result.error) {
      const code = result.error.error.code as string
      if (code === 'account_changed') { confirmation.value = ''; accountChanged.value = true }
      error.value = errorMessage(language.value, code)
      return
    }
    session.$reset()
    session.status = 'signed-out'
    await navigateTo('/?account=deleted')
  } catch { error.value = errorMessage(language.value, 'generic') }
  finally { busy.value = false }
}
</script>
<template>
  <dialog ref="dialog" class="card account-dialog" aria-labelledby="delete-dialog-title" @cancel.prevent="close" @keydown="wrapDialogFocus">
    <h2 id="delete-dialog-title">{{ messages.deleteAccount }}</h2><p>{{ messages.deletePrompt }}</p>
    <form @submit.prevent="remove">
      <label>{{ messages.email }} <input v-model="confirmation" class="input" autocomplete="off"></label>
      <p v-if="error" class="notice" role="alert" aria-live="assertive">{{ error }}</p>
      <div class="account-dialog-actions"><button class="btn btn-danger" type="submit" :disabled="confirmation !== email || busy || accountChanged">{{ messages.deleteAccount }}</button><button class="btn btn-secondary" type="button" @click="close">{{ messages.close }}</button></div>
    </form>
  </dialog>
</template>
