<script setup lang="ts">
import { messagesFor, shellMessages, type Language } from '../i18n'
const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
const unavailable = useState<boolean>('service-unavailable', () => false)
const messages = computed(() => messagesFor(language.value))
const shell = computed(() => shellMessages(language.value))
function retry() { window.location.reload() }
useHead(() => unavailable.value ? { title: `${shell.value.unavailableTitle} · Tra Chuẩn` } : {})
</script>
<template>
  <NuxtLayout :name="unavailable ? 'default' : undefined"><div v-if="unavailable" class="notice search-state" role="alert"><h1>{{ shell.unavailableTitle }}</h1><p>{{ messages.errors.unavailable }}</p><button type="button" class="btn btn-secondary" @click="retry">{{ shell.retry }}</button></div><NuxtPage v-else /></NuxtLayout>
</template>
