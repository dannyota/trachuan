<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
</script>
<template><div><h1>{{ messages.privacy }}</h1><LegalFacts /></div></template>
