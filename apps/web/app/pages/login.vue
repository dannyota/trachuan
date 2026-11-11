<script setup lang="ts">
import { loginError, messagesFor, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const route = useRoute()
const error = computed(() => route.query.error === undefined ? null : loginError(language.value, route.query.error))
</script>
<template><div><h1>{{ messages.login }}</h1><p v-if="error">{{ error }}</p><a href="/api/v1/auth/google/start?next=%2F">{{ messages.signIn }} Google</a></div></template>
