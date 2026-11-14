<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
import { legalCopy } from '../../i18n/legal'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const copy = computed(() => legalCopy(language.value))
</script>
<template>
  <div class="container page-content legal-page">
    <h1>{{ messages.terms }}</h1>
    <div class="legal-facts">
      <p class="legal-updated">{{ copy.updated }}</p>
      <section v-for="[heading, text] in copy.terms" :key="heading">
        <h2>{{ heading }}</h2><p>{{ text }}</p>
        <p v-if="heading === 'Phạm vi' || heading === 'Coverage'"><NuxtLink to="/sources">{{ messages.sources }}</NuxtLink></p>
        <p v-if="heading === 'Xóa tài khoản' || heading === 'Account deletion'"><NuxtLink to="/privacy">{{ messages.privacy }}</NuxtLink></p>
      </section>
    </div>
  </div>
</template>
