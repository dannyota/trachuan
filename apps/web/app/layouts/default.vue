<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'

const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
const messages = computed(() => messagesFor(language.value))
const route = useRoute()
const signedIn = ref(false)
const signIn = computed(() => `/api/v1/auth/google/start?next=${encodeURIComponent(route.path)}`)

onMounted(async () => {
  try {
    const response = await fetch('/api/v1/me', { credentials: 'same-origin' })
    signedIn.value = response.ok
  } catch {
    signedIn.value = false
  }
})
function toggleLanguage() {
  const next = language.value === 'vi' ? 'en' : 'vi'
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(location.hostname)
  const secure = location.protocol === 'http:' && loopback ? '' : '; Secure'
  document.cookie = `lang=${next}; Path=/; Max-Age=31536000; SameSite=Lax${secure}`
  location.reload()
}
useHead(() => ({ htmlAttrs: { lang: language.value } }))
</script>

<template>
  <div>
    <header>
      <NuxtLink to="/">trachuan</NuxtLink>
      <nav>
        <NuxtLink to="/connect">{{ messages.connect }}</NuxtLink>
        <NuxtLink to="/sources">{{ messages.sources }}</NuxtLink>
      </nav>
      <button type="button" @click="toggleLanguage">{{ messages.language }}</button>
      <NuxtLink v-if="signedIn" to="/account">{{ messages.account }}</NuxtLink>
      <a v-else :href="signIn">{{ messages.signIn }}</a>
    </header>
    <main><slot /></main>
  </div>
</template>
