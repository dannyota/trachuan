<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ text: string }>()
const pieces = computed(() => props.text.match(/\s+|\S+/g) || [])
</script>
<template><template v-for="(piece, index) in pieces" :key="index"><template v-if="/^\s+$/.test(piece)">{{ piece }}</template><span v-else class="command-token">{{ piece }}</span></template></template>
