<script setup lang="ts">
import { messagesFor, type Language } from '../../i18n'
import { loginUrl } from '../api/next'
import { useSessionStore } from '../stores/session'

const requestLanguage = useRequestHeader('x-trachuan-language')
const language = useState<Language>('language', () => requestLanguage === 'en' ? 'en' : 'vi')
const messages = computed(() => messagesFor(language.value))
const route = useRoute()
const session = useSessionStore()
const api = useApi()
const signIn = computed(() => loginUrl(route.fullPath))

onMounted(async () => {
  try {
    if (session.status === 'unknown') await api.loadSession()
  } catch {
    session.status = 'signed-out'
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
      <a href="/">trachuan</a>
      <nav>
        <NuxtLink to="/connect">{{ messages.connect }}</NuxtLink>
        <a href="/sources">{{ messages.sources }}</a>
      </nav>
      <button type="button" @click="toggleLanguage">{{ messages.language }}</button>
      <NuxtLink v-if="session.status === 'signed-in'" to="/account">{{ messages.account }}</NuxtLink>
      <a v-else :href="signIn">{{ messages.signIn }}</a>
    </header>
    <main><slot /></main>
  </div>
</template>
