<script setup lang="ts">
import { takeLinkToken } from '../utils/account-security'
import { useAccountLink } from '../composables/useAccountLink'

const { labels, link, password, error, busy, saved, signInHref, save } = useAccountLink(import.meta.client ? takeLinkToken(window.location, window.history, useRouter().options.history) : '')
</script>
<template>
  <div class="container login-page"><section class="card login-card">
    <h1>{{ link?.purpose === 'reset' ? labels.reset : labels.setPassword }}</h1>
    <p v-if="error" class="notice" role="alert">{{ error }}</p>
    <p v-if="saved" role="status">{{ labels.saved }}</p>
    <form v-if="link" class="auth-form" @submit.prevent="save()">
      <p>{{ link.email }}</p><p id="password-help">{{ labels.passwordHelp }}</p>
      <label>{{ labels.newPassword }}<input v-model="password" class="input" type="password" autocomplete="new-password" aria-describedby="password-help" required></label>
      <button class="btn btn-primary" :disabled="busy">{{ labels.savePassword }}</button>
    </form>
    <NuxtLink :to="signInHref">{{ labels.signIn }}</NuxtLink>
  </section></div>
</template>
