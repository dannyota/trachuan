<script setup lang="ts">
import { computed } from 'vue'
import type { Language } from '../../i18n'
import { publicHead } from '../../i18n/public-head'
import { takeLinkToken } from '../utils/account-security'
import { useAccountLink } from '../composables/useAccountLink'

const { labels, link, password, error, busy, saved, signInHref, save } = useAccountLink(import.meta.client ? takeLinkToken(window.location, window.history, useRouter().options.history) : '')
const language = useState<Language>('language', () => 'vi')
const reset = computed(() => link.value?.purpose === 'reset' || saved.value)
useHead(() => ({ title: `${publicHead(language.value, reset.value ? '/reset' : '/link')!.title} · Tra Chuẩn` }))
</script>
<template>
  <div class="container login-page"><section class="card login-card">
    <h1>{{ reset ? labels.reset : labels.setPassword }}</h1>
    <p v-if="error" class="notice" role="alert">{{ error }}</p>
    <p v-if="saved" role="status">{{ labels.saved }}</p>
    <form v-if="link" class="auth-form" @submit.prevent="save()">
      <input :value="link.email" class="sr-only" type="text" name="username" autocomplete="username" :aria-label="labels.email" tabindex="-1" readonly>
      <p>{{ link.email }}</p><p id="password-help">{{ labels.passwordHelp }}</p>
      <label>{{ labels.newPassword }}<input v-model="password" class="input" type="password" autocomplete="new-password" aria-describedby="password-help" required></label>
      <button class="btn btn-primary" :disabled="busy">{{ labels.savePassword }}</button>
    </form>
    <NuxtLink :to="signInHref">{{ labels.signIn }}</NuxtLink>
  </section></div>
</template>
