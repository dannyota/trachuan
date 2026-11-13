<script setup lang="ts">
import { loginError, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => pagesMessages(language.value))
const route = useRoute()
const error = computed(() => route.query.error === undefined ? null : loginError(language.value, route.query.error))
</script>
<template><div class="container login-page"><div class="card login-card"><SealLogo /><h1>{{ labels.loginTitle }}</h1><p>{{ labels.loginIntro }}</p><p v-if="error" class="notice notice--warn" role="status" aria-live="polite">{{ error }}</p><a class="btn btn-primary" href="/api/v1/auth/google/start?next=%2F">{{ labels.loginGoogle }}</a></div></div></template>
