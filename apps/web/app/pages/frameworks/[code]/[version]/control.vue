<script setup lang="ts">
import { controlHandle } from '../../../../utils/framework-handles'
definePageMeta({ signedIn: true, key: route => route.fullPath })
const route = useRoute()
const { notFound } = useFrameworkPage()
const citation = typeof route.query.citation === 'string' ? route.query.citation : ''
const handle = controlHandle(String(route.params.code), String(route.params.version), citation)
if (!handle) notFound()
</script>
<template><main><FrameworkRead v-if="handle" :handle="handle" kind="control" /></main></template>
