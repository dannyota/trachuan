<script setup lang="ts">
import { computed } from 'vue'
import type { Language } from '../../i18n'
import { frameworkMessages } from '../../i18n/frameworks'
const props = defineProps<{ current: 'law' | 'frameworks' }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => frameworkMessages(language.value))
const router = useRouter()
function change(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  if (value !== props.current) void router.push(value === 'law' ? '/vn/search' : '/frameworks/search')
}
</script>
<template><label><span class="visually-hidden">{{ labels.filter }}</span><select class="input" :value="current" @change="change"><option value="law">{{ labels.law }}</option><option value="frameworks">{{ labels.frameworks }}</option></select></label></template>
