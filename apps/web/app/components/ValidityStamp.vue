<script setup lang="ts">
import { statusMessage, type Language } from '../../i18n'
const props = defineProps<{ status?: string }>()
const language = useState<Language>('language', () => 'vi')
const kind = computed(() => ({ in_force: 'in-force', partial: 'partial', not_yet: 'not-yet', expired: 'expired', suspended: 'suspended' }[props.status ?? ''] ?? 'unknown'))
</script>
<template><span class="stamp" :class="`stamp--${kind}`">{{ statusMessage(language, status) }}</span></template>
