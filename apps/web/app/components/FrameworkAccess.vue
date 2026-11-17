<script setup lang="ts">
import { computed } from 'vue'
import { Lock, LockOpen } from '@lucide/vue'
import { frameworkMessages } from '../../i18n/frameworks'
import type { Language } from '../../i18n'
defineProps<{ access: 'free' | 'licensed' }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => frameworkMessages(language.value))
</script>
<template><span class="fw-access"><Lock v-if="access === 'licensed'" :size="15" aria-hidden="true" /><LockOpen v-else :size="15" aria-hidden="true" />{{ labels[access] }}</span></template>
