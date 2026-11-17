<script setup lang="ts">
import { messagesFor, shellMessages, type Language } from '../../i18n'
const props = defineProps<{ text: string, label: string }>()
const language = useState<Language>('language', () => 'vi')
const outcome = ref<'idle' | 'copied' | 'failed'>('idle')
const announcement = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let request = 0
const shown = computed(() => outcome.value === 'copied' ? shellMessages(language.value).copied : outcome.value === 'failed' ? shellMessages(language.value).copyFailed : props.label)
async function copy() {
  const current = ++request
  clearTimeout(timer)
  outcome.value = 'idle'
  let succeeded = false
  try { await navigator.clipboard.writeText(props.text); succeeded = true } catch { succeeded = false }
  if (current !== request) return
  outcome.value = succeeded ? 'copied' : 'failed'
  announcement.value = succeeded ? shellMessages(language.value).copied : messagesFor(language.value).errors.generic
  timer = setTimeout(() => { outcome.value = 'idle'; announcement.value = '' }, 2000)
}
onUnmounted(() => { request++; clearTimeout(timer) })
</script>
<template><span><button type="button" class="ghost-button" @click="copy"><svg v-if="outcome === 'copied'" data-icon="check" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.5"><path d="m3 8 3 3 7-7" /></svg><svg v-else-if="outcome === 'failed'" data-icon="alert" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="6" /><path d="M8 5v3.5M8 10.5v.5" /></svg><svg v-else data-icon="copy" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="5" y="5" width="8" height="9" rx="1" /><path d="M11 5V3a1 1 0 0 0-1-1H3a1 1 0 0 0-1 1v8a1 1 0 0 0 1 1h2" /></svg><span class="copy-label"><span>{{ shown }}</span><span class="copy-label-size" :data-label="label" aria-hidden="true" /><span class="copy-label-size" :data-label="shellMessages(language).copied" aria-hidden="true" /><span class="copy-label-size" :data-label="shellMessages(language).copyFailed" aria-hidden="true" /></span></button><span class="visually-hidden" aria-live="polite">{{ announcement }}</span></span></template>
