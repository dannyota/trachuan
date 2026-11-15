<script setup lang="ts">
import CopyButton from './CopyButton.vue'
import { computed, ref } from 'vue'
import { authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import { recoveryText } from '../utils/account-security'
const props = defineProps<{ codes: string[] }>()
const emit = defineEmits<{ close: [] }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const saved = ref(false)
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
  <div class="recovery-panel"><p>{{ labels.recoveryOnce }}</p><p>{{ labels.recoveryUse }}</p>
    <ol class="recovery-codes"><li v-for="(line, index) in text.trimEnd().split('\n')" :key="index"><span>{{ line.slice(4) }}</span></li></ol>
    <div class="auth-actions"><CopyButton :text="text" :label="labels.copy" /><button class="btn btn-secondary" type="button" @click="download">{{ labels.download }}</button></div>
    <label class="recovery-confirm"><input v-model="saved" type="checkbox"> {{ labels.recoverySaved }}</label>
    <button class="btn btn-primary" type="button" :disabled="!saved" @click="emit('close')">{{ labels.recoveryClose }}</button>
  </div>
</template>
