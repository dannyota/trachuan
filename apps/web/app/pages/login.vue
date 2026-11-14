<script setup lang="ts">
import { computed, ref } from 'vue'
import { loginError, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
import { authError, authMessages } from '../../i18n/auth'
import { safeNext } from '../api/next'

const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const pages = computed(() => pagesMessages(language.value))
const route = useRoute()
const next = computed(() => safeNext(typeof route.query.next === 'string' ? route.query.next : '/'))
const google = computed(() => `/api/v1/auth/google/start?next=${encodeURIComponent(next.value)}`)
const urlError = computed(() => route.query.error === undefined ? null : loginError(language.value, route.query.error))
const email = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)
const client = useAuthApi()
const secrets = usePageSecrets(() => { password.value = '' })
async function signIn() {
  const signal = secrets.signal
  if (busy.value || signal.aborted) return
  busy.value = true; error.value = ''
  try {
    const result = await client.POST('/api/v1/auth/password', { params: { header: { Origin: window.location.origin } }, body: { email: email.value, password: password.value, next: next.value }, signal })
    if (signal.aborted) return
    if (result.data) window.location.assign('factor_required' in result.data ? '/login/factor' : safeNext(result.data.next))
    else error.value = authError(language.value, result.error?.error.code, result.response.status)
  } catch { if (!signal.aborted) error.value = labels.value.generic }
  finally { busy.value = false; password.value = '' }
}
</script>
<template>
  <div class="container login-page"><div class="card login-card">
    <SealLogo /><h1>{{ pages.loginTitle }}</h1><p>{{ pages.loginIntro }}</p>
    <p v-if="urlError" class="notice notice--warn" role="status">{{ urlError }}</p>
    <a class="btn btn-primary" :href="google">{{ labels.google }}</a>
    <form class="auth-form" @submit.prevent="signIn">
      <label>{{ labels.email }}<input v-model="email" class="input" type="email" autocomplete="username" required></label>
      <label>{{ labels.password }}<input v-model="password" class="input" type="password" autocomplete="current-password" required></label>
      <p v-if="error" class="notice" role="alert">{{ error }}</p>
      <button class="btn btn-primary" :disabled="busy">{{ labels.signIn }}</button>
    </form>
    <div class="auth-actions"><NuxtLink to="/signup">{{ labels.signup }}</NuxtLink><NuxtLink to="/reset">{{ labels.reset }}</NuxtLink></div>
  </div></div>
</template>
