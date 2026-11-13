<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => pagesMessages(language.value))
</script>
<template><div class="legal-facts"><p class="legal-updated">{{ labels.updated }}</p><section><h2>{{ labels.evidenceHeading }}</h2><p>{{ messages.legalEvidence }}</p></section><section><h2>{{ labels.identityHeading }}</h2><p>{{ messages.legalIdentity }}</p></section><section><h2>{{ labels.storageHeading }}</h2><p>{{ messages.legalStorage }}</p></section><section><h2>{{ labels.questionsHeading }}</h2><p>{{ messages.legalQuestions }}</p></section><section><h2>{{ labels.deletionHeading }}</h2><p>{{ messages.legalDeletion }}</p></section></div></template>
