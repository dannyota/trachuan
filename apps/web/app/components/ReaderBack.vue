<script setup lang="ts">
import { readerMessages } from '../../i18n/reader'
import type { Language } from '../../i18n'

const language = useState<Language>('language', () => 'vi')
const m = computed(() => readerMessages(language.value))
function back(event: MouseEvent) {
  const previous = window.history.state?.back
  if (typeof previous !== 'string') return
  try {
    const url = new URL(previous, window.location.origin)
    if (url.origin === window.location.origin && url.pathname === '/vn/search') { event.preventDefault(); window.history.back() }
  } catch { return }
}
</script>
<template><a href="/vn/search" class="reader-back" @click="back">{{ m.back }}</a></template>
