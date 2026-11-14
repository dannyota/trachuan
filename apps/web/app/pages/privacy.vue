<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
import { legalCopy } from '../../i18n/legal'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const copy = computed(() => legalCopy(language.value))
</script>
<template>
  <div class="container page-content legal-page">
    <h1>{{ messages.privacy }}</h1>
    <div class="legal-facts">
      <p class="legal-updated">{{ copy.updated }}</p>
      <section v-for="[heading, text] in copy.privacy" :key="heading">
        <h2>{{ heading }}</h2><p>{{ text }}</p>
        <p v-if="heading === 'Chính sách dữ liệu Google' || heading === 'Google data policy'"><a href="https://developers.google.com/terms/api-services-user-data-policy">Google API Services User Data Policy</a></p>
      </section>
    </div>
  </div>
</template>
