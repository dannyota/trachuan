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
      <label for="username">{{ labels.email }}<input id="username" :value="link.email" class="input" type="email" name="username" autocomplete="username" readonly></label>
      <p id="password-help">{{ labels.passwordHelp }}</p>
      <label for="new-password">{{ labels.newPassword }}<input id="new-password" v-model="password" class="input" type="password" name="new-password" autocomplete="new-password" minlength="15" passwordrules="minlength: 15; maxlength: 128;" aria-describedby="password-help" required></label>
      <button class="btn btn-primary" type="submit" :disabled="busy">{{ labels.savePassword }}</button>
    </form>
    <NuxtLink :to="signInHref">{{ link?.purpose === 'add_password' ? labels.backToAccount : labels.signIn }}</NuxtLink>
  </section></div>
</template>
