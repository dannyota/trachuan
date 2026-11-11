<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const endpoint = `${useRuntimeConfig().public.siteOrigin.replace(/\/$/, '')}/mcp`
const cursor = computed(() => JSON.stringify({ mcpServers: { trachuan: { url: endpoint } } }, null, 2))
</script>
<template><div><h1>{{ messages.connect }}</h1><p>{{ messages.connectIntro }} <code>{{ endpoint }}</code></p>
  <section><h2>Claude.ai</h2><p>{{ messages.claudeSteps }}</p></section>
  <section><h2>ChatGPT</h2><p>{{ messages.chatgptSteps }}</p></section>
  <section><h2>Claude Code</h2><p>{{ messages.claudeCodeSteps }}</p><pre>claude mcp add --transport http trachuan {{ endpoint }}</pre></section>
  <section><h2>Cursor</h2><p>{{ messages.cursorSteps }}</p><pre>{{ cursor }}</pre></section>
</div></template>
