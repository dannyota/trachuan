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
<template><a href="/vn/search" class="reader-back" @click="back"><svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M7 3 2 8l5 5M2 8h12" /></svg> {{ m.back }}</a></template>
