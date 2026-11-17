<script setup lang="ts">
import { versionHandle } from '../../../../utils/framework-handles'
definePageMeta({ signedIn: true, key: route => route.fullPath })
const route = useRoute()
const { notFound } = useFrameworkPage()
const handle = versionHandle(String(route.params.code), String(route.params.version))
if (!handle) notFound()
</script>
<template><main><FrameworkRead v-if="handle" :handle="handle" kind="version" /><FrameworkGaps v-if="handle" :handle="handle" /></main></template>
