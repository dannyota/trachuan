<script setup lang="ts">
import { homeMessages } from '../../i18n/home'
import type { Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => homeMessages(language.value))
const endpoint = `${useRuntimeConfig().public.siteOrigin.replace(/\/$/, '')}/mcp`
const command = `claude mcp add --transport http trachuan ${endpoint}`
</script>
<template>
  <section class="home-connect" aria-labelledby="home-connect-title">
    <div class="container home-connect-inner">
      <div class="home-connect-heading"><div><h2 id="home-connect-title">{{ labels.connectTitle }}</h2><p>{{ labels.connectText }}</p></div><div class="home-address"><code :aria-label="labels.address">{{ endpoint }}</code><CopyButton :text="endpoint" :label="labels.copy" /></div></div>
      <div class="home-connect-cards">
        <section class="card home-connect-card"><h3>Claude.ai</h3><p>{{ labels.claude }}</p></section>
        <section class="card home-connect-card"><h3>ChatGPT</h3><p>{{ labels.chat }}</p></section>
        <section class="card home-connect-card"><h3>Claude Code</h3><p>{{ labels.code }}</p><pre tabindex="0" :aria-label="labels.code">{{ command }}</pre><CopyButton :text="command" :label="labels.copy" /></section>
      </div>
      <NuxtLink to="/connect">{{ labels.connectLink }}</NuxtLink>
    </div>
  </section>
</template>
