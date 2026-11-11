<script setup lang="ts">
import { getCoverage } from '../api/coverage'
import { messagesFor, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi()
const { data: status } = await useAsyncData('coverage-sources', () => import.meta.server ? getCoverage(() => api.client.GET('/api/v1/public/status')).then(status => ({ status })) : Promise.resolve({ status: null }), { server: true, default: () => ({ status: null }) })
</script>
<template><div><h1>{{ messages.sources }}</h1><CoverageFacts :status="status.status" /></div></template>
