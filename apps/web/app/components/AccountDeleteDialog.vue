<script setup lang="ts">
import { messagesFor, errorMessage, type Language } from '../../i18n'
import { accountActions } from '../api/account-actions'
import { useSessionStore } from '../stores/session'

const props = defineProps<{ email: string }>()
const emit = defineEmits<{ close: [] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const dialog = ref<HTMLDialogElement | null>(null)
onMounted(() => dialog.value?.showModal())
const confirmation = ref('')
const error = ref('')
const busy = ref(false)
const api = useApi({ userAction: true })
const session = useSessionStore()
async function remove() {
  if (confirmation.value !== props.email || busy.value) return
  busy.value = true
  try {
    const result = await accountActions(api.client).deleteAccount()
    if (result.error) { error.value = errorMessage(language.value, result.error.error.code); return }
    session.$reset()
    session.status = 'signed-out'
    await navigateTo('/?account=deleted')
  } catch { error.value = errorMessage(language.value, 'generic') }
  finally { busy.value = false }
}
</script>
<template>
  <dialog ref="dialog" @cancel.prevent="emit('close')">
    <h2>{{ messages.deleteAccount }}</h2><p>{{ messages.deletePrompt }}</p>
    <form @submit.prevent="remove">
      <label>{{ messages.email }} <input v-model="confirmation" autocomplete="off"></label>
      <button type="submit" :disabled="confirmation !== email || busy">{{ messages.deleteAccount }}</button>
    </form>
    <p v-if="error" role="alert">{{ error }}</p>
    <button type="button" @click="emit('close')">{{ messages.close }}</button>
  </dialog>
</template>
