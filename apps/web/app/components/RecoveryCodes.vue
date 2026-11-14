<script setup lang="ts">
import CopyButton from './CopyButton.vue'
import { computed } from 'vue'
import { authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import { recoveryText } from '../utils/account-security'
const props = defineProps<{ codes: string[] }>()
const emit = defineEmits<{ close: [] }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const text = computed(() => recoveryText(props.codes))
function download() {
  const url = URL.createObjectURL(new Blob([text.value], { type: 'text/plain;charset=utf-8' }))
  const anchor = document.createElement('a')
  anchor.href = url; anchor.download = 'trachuan-recovery-codes.txt'
  document.body.append(anchor); anchor.click(); anchor.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}
</script>
<template>
  <div><p>{{ labels.recoveryOnce }}</p><pre class="recovery-codes">{{ text }}</pre>
    <div class="auth-actions"><CopyButton :text="text" :label="labels.copy" /><button class="btn btn-secondary" type="button" @click="download">{{ labels.download }}</button><button class="btn btn-secondary" type="button" @click="emit('close')">{{ labels.close }}</button></div>
  </div>
</template>
