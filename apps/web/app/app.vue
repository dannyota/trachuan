<script setup lang="ts">
import { messagesFor, type Language } from '../i18n'
const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
const unavailable = useState<boolean>('service-unavailable', () => false)
const messages = computed(() => messagesFor(language.value))
</script>
<template>
  <NuxtLayout><p v-if="unavailable">{{ messages.errors.unavailable }}</p><NuxtPage v-else /></NuxtLayout>
</template>
