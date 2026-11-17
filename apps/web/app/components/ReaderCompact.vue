<script setup lang="ts">
import { computed, ref } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import type { ReaderPosition } from '../utils/reader-position'
import { readerMessages } from '../../i18n/reader'
import type { Language } from '../../i18n'

const props = defineProps<{ position: ReaderPosition, units: components['schemas']['Unit'][], showTop: boolean }>()
const emit = defineEmits<{ open: [], top: [] }>()
const language = useState<Language>('language', () => 'vi')
const label = computed(() => props.units.find(unit => unit.anchor === props.position.article.value)?.label || '')
const button = ref<HTMLButtonElement>()
defineExpose({ focus: () => button.value?.focus() })
</script>
<template>
  <div class="reader-compact"><span>{{ label }}</span><button v-if="showTop" class="btn btn-secondary reader-top" type="button" @click="emit('top')">{{ readerMessages(language).backToTop }}</button><button ref="button" class="btn btn-secondary" type="button" @click="emit('open')">{{ readerMessages(language).contents }}</button></div>
</template>
