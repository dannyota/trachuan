<script setup lang="ts">
import { onBeforeRouteLeave } from 'vue-router'
import { nextTick } from 'vue'
import CopyButton from './CopyButton.vue'
import ReauthForm from './ReauthForm.vue'
import { useReauth } from '../composables/useReauth'
import { wrapDialogFocus } from '../utils/dialog-focus'
import { accountActions } from '../api/account-actions'
import type { Action } from '../api/client'
import { errorMessage, messagesFor, type Language } from '../../i18n'

const props = defineProps<{ action: Action }>()
const emit = defineEmits<{ close: [], created: [] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const dialog = ref<HTMLDialogElement | null>(null)
const copyArea = ref<HTMLElement | null>(null)
let opener: HTMLElement | null = null
onMounted(() => {
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  dialog.value?.showModal()
  window.addEventListener('pagehide', close)
  window.addEventListener('pageshow', restored)
})
const api = useApi({ userAction: true })
const reauth = useReauth()
const actions = accountActions(api.forAction(props.action))
const name = ref('')
const days = ref(90)
const token = ref('')
const error = ref('')
const busy = ref(false)
let active = true
function clear() { active = false; token.value = ''; reauth.cancel() }
function close() { clear(); dialog.value?.close(); if (opener?.isConnected) opener.focus(); emit('close') }
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
    const result = await actions.createToken(name.value, days.value)
    if (!active) return
    if (result.data) { token.value = result.data.token; emit('created'); await nextTick(); if (active) copyArea.value?.querySelector('button')?.focus() }
    else if (!result.error || !reauth.requireProof(result.error, props.action, false, create)) error.value = errorMessage(language.value, result.error?.error.code || 'generic')
  } catch { if (active) error.value = errorMessage(language.value, 'generic') }
  finally { busy.value = false }
}
</script>
<template>
  <dialog ref="dialog" class="card account-dialog" aria-labelledby="token-dialog-title" @cancel.prevent="close" @keydown="wrapDialogFocus">
    <h2 id="token-dialog-title">{{ messages.createToken }}</h2>
    <ReauthForm v-if="reauth.challenge.value" v-bind="reauth.challenge.value" @verified="reauth.verified" @cancel="close" />
    <form v-else-if="!token" @submit.prevent="create">
      <label>{{ messages.name }} <input v-model="name" class="input" required maxlength="64"></label>
      <label>{{ messages.days }} <input v-model.number="days" class="input" type="number" min="1" max="365" required></label>
      <p v-if="error" class="notice" role="alert" aria-live="assertive">{{ error }}</p>
      <div class="account-dialog-actions"><button class="btn btn-primary" type="submit" :disabled="busy">{{ messages.create }}</button><button class="btn btn-secondary" type="button" @click="close">{{ messages.close }}</button></div>
    </form>
    <div v-else><p>{{ messages.tokenOnce }}</p><p class="token-value">{{ token }}</p><div ref="copyArea" class="account-dialog-actions"><CopyButton :text="token" :label="messages.copy" /><button class="btn btn-secondary" type="button" @click="close">{{ messages.close }}</button></div></div>
  </dialog>
</template>
