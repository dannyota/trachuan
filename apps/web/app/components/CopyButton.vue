<script setup lang="ts">
import { messagesFor, shellMessages, type Language } from '../../i18n'
const props = defineProps<{ text: string, label: string }>()
const language = useState<Language>('language', () => 'vi')
const feedback = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
async function copy() {
  clearTimeout(timer)
  try { await navigator.clipboard.writeText(props.text); feedback.value = shellMessages(language.value).copied }
  catch { feedback.value = messagesFor(language.value).errors.generic }
  timer = setTimeout(() => { feedback.value = '' }, 2000)
}
onUnmounted(() => clearTimeout(timer))
</script>
<template><span><button type="button" class="btn btn-secondary" @click="copy">{{ label }}</button><span aria-live="polite">{{ feedback }}</span></span></template>
