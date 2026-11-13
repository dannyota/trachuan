<script setup lang="ts">
import { getCoverage } from '../api/coverage'
import { messagesFor, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => pagesMessages(language.value))
const api = useApi()
const { data: status } = await useAsyncData('coverage-sources', () => import.meta.server ? getCoverage(() => api.client.GET('/api/v1/public/status')).then(status => ({ status })) : Promise.resolve({ status: null }), { server: true, default: () => ({ status: null }) })
</script>
<template><div class="container page-content"><header class="page-intro"><h1>{{ messages.sources }}</h1><p>{{ labels.sourcesIntro }}</p></header><CoverageFacts :status="status.status" show-gaps><section class="page-section"><h2>{{ messages.sources }}</h2><div class="source-list"><div><h3><a href="https://vbpl.vn/">vbpl.vn</a></h3><p>{{ labels.vbpl }}</p></div><div><h3><a href="https://chinhphu.vn/">Cổng thông tin điện tử Chính phủ</a></h3><p>{{ labels.government }}</p></div></div></section></CoverageFacts></div></template>
