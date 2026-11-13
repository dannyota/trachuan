<script setup lang="ts">
import type { Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => pagesMessages(language.value))
const endpoint = `${useRuntimeConfig().public.siteOrigin.replace(/\/$/, '')}/mcp`
const command = `claude mcp add --transport http trachuan ${endpoint}`
const cursor = JSON.stringify({ mcpServers: { trachuan: { url: endpoint } } }, null, 2)
</script>
<template>
  <div class="container page-content">
    <header class="page-intro"><h1>{{ labels.connectTitle }}</h1><p>{{ labels.connectIntro }}</p></header>
    <section class="server-address" :aria-label="labels.serverAddress"><h2>{{ labels.serverAddress }}</h2><div class="page-code"><pre>{{ endpoint }}</pre><CopyButton :text="endpoint" :label="labels.copy" /></div></section>
    <div class="connect-grid">
      <section class="card connect-card"><h2>Claude.ai</h2><ol><li>{{ labels.claude1 }}</li><li>{{ labels.claude2 }}</li><li>{{ labels.claude3 }}</li></ol></section>
      <section class="card connect-card"><h2>ChatGPT</h2><ol><li>{{ labels.chat1 }}</li><li>{{ labels.chat2 }}</li><li>{{ labels.chat3 }}</li></ol></section>
      <section class="card connect-card"><h2>Claude Code</h2><p>{{ labels.codeIntro }}</p><div class="page-code"><pre>{{ command }}</pre><CopyButton :text="command" :label="labels.copy" /></div></section>
      <section class="card connect-card"><h2>{{ labels.cursorTitle }}</h2><p>{{ labels.cursorBefore }} <NuxtLink to="/account">{{ labels.tokenLink }}</NuxtLink>{{ labels.cursorAfter }}</p><div class="page-code"><pre>{{ cursor }}</pre><CopyButton :text="cursor" :label="labels.copy" /></div></section>
    </div>
    <section class="connect-tools"><h2>{{ labels.tools }}</h2><div><code>search</code><p>{{ labels.searchTool }}</p></div><div><code>read</code><p>{{ labels.readTool }}</p></div><div><code>taxonomy</code><p>{{ labels.taxonomyTool }}</p></div><div><code>report_issue</code><p>{{ labels.reportTool }}</p></div></section>
  </div>
</template>
