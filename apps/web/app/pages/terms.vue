<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
</script>
<template><div class="container page-content legal-page"><h1>{{ messages.terms }}</h1><LegalFacts /></div></template>
