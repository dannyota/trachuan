<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
</script>
<template><div><p>{{ messages.legalEvidence }}</p><p>{{ messages.legalIdentity }}</p><p>{{ messages.legalStorage }}</p><p>{{ messages.legalQuestions }}</p><p>{{ messages.legalDeletion }}</p></div></template>
