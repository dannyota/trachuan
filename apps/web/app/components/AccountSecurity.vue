<script setup lang="ts">
import AccountPassword from './AccountPassword.vue'
import AccountAuthenticator from './AccountAuthenticator.vue'
import { computed } from 'vue'
import { authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
const emit = defineEmits<{ changed: [signal: AbortSignal] }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
</script>
<template><section class="card account-security"><h2>{{ labels.security }}</h2><AccountPassword @changed="emit('changed', $event)" /><AccountAuthenticator @changed="emit('changed', $event)" /></section></template>
