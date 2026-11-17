<script setup lang="ts">
defineProps<{ text: string }>()
const address = 'support@trachuan.vn'
</script>
<template><template v-for="(part, index) in text.split(address)" :key="index"><a v-if="index" :href="`mailto:${address}`">{{ address }}</a>{{ part }}</template></template>
