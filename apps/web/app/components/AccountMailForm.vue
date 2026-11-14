<script setup lang="ts">
import { computed, ref } from 'vue'
import { authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'

const props = defineProps<{ purpose: 'signup' | 'reset' }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const client = useAuthApi()
const email = ref('')
const sent = ref(false)
const busy = ref(false)
const limited = ref(false)
const secrets = usePageSecrets(() => { email.value = '' })
async function send() {
  const signal = secrets.signal
  if (busy.value || signal.aborted) return
  busy.value = true; limited.value = false
  try {
    const result = await client.POST(props.purpose === 'signup' ? '/api/v1/auth/signup' : '/api/v1/auth/reset', { params: { header: { Origin: window.location.origin } }, body: { email: email.value }, signal })
    if (signal.aborted) return
    if (result.response.status === 429) limited.value = true
    else sent.value = true
  } catch { if (!signal.aborted) sent.value = true }
  finally { busy.value = false }
}
</script>
<template>
  <div class="container login-page"><section class="card login-card">
    <h1>{{ purpose === 'signup' ? labels.signup : labels.reset }}</h1>
    <p v-if="sent" role="status">{{ labels.mail }}</p>
    <form v-else class="auth-form" @submit.prevent="send">
      <label>{{ labels.email }}<input v-model="email" class="input" type="email" autocomplete="email" required></label>
      <p v-if="limited" class="notice" role="alert">{{ labels.rateLimited }}</p>
      <button class="btn btn-primary" :disabled="busy">{{ labels.send }}</button>
    </form>
    <NuxtLink to="/login">{{ labels.signIn }}</NuxtLink>
  </section></div>
</template>
