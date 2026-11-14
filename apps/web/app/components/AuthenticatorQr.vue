<script setup lang="ts">
import { computed } from 'vue'
import { encode } from 'uqr'
import { qrGeometry } from '../utils/account-security'
const props = defineProps<{ uri: string, label: string }>()
const geometry = computed(() => qrGeometry(encode(props.uri, { border: 0 }).data))
</script>
<template>
  <svg class="auth-qr" :viewBox="`0 0 ${geometry.size} ${geometry.size}`" role="img" :aria-label="label" shape-rendering="crispEdges">
    <rect width="100%" height="100%" fill="white" /><path :d="geometry.path" fill="black" />
  </svg>
</template>
