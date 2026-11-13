<script setup lang="ts">
import { shellMessages, type Language } from '../../i18n'
import { loginUrl } from '../api/next'
import { useSessionStore } from '../stores/session'

withDefaults(defineProps<{ compact?: boolean }>(), { compact: false })
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => shellMessages(language.value))
const route = useRoute()
const session = useSessionStore()
const open = ref(false)
const button = ref<HTMLButtonElement | null>(null)
const header = ref<HTMLElement | null>(null)
const links = computed(() => [
  { path: '/vn/search', label: messages.value.search }, { path: '/vn/topics', label: messages.value.topics },
  { path: '/connect', label: messages.value.connect }, { path: '/sources', label: messages.value.sources },
])
function closeMenu() { open.value = false; button.value?.focus() }
function onKey(event: KeyboardEvent) { if (open.value && event.key === 'Escape') closeMenu() }
function onOutside(event: MouseEvent) { if (open.value && !header.value?.contains(event.target as Node)) closeMenu() }
function toggleLanguage() {
  const next = language.value === 'vi' ? 'en' : 'vi'
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(location.hostname)
  const secure = location.protocol === 'http:' && loopback ? '' : '; Secure'
  document.cookie = `lang=${next}; Path=/; Max-Age=31536000; SameSite=Lax${secure}`
  location.reload()
}
onMounted(() => { document.addEventListener('keydown', onKey); document.addEventListener('click', onOutside) })
onUnmounted(() => { document.removeEventListener('keydown', onKey); document.removeEventListener('click', onOutside) })
</script>
<template>
  <header ref="header" class="site-header">
    <div class="container header-inner">
      <NuxtLink class="site-logo" to="/"><SealLogo /><span>Tra Chuẩn</span></NuxtLink>
      <slot />
      <button ref="button" class="menu-button" type="button" :aria-label="messages.menu" :aria-expanded="open" aria-controls="site-menu" @click="open = !open"><svg width="22" height="22" viewBox="0 0 22 22" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2"><line x1="3" y1="5" x2="19" y2="5" /><line x1="3" y1="11" x2="19" y2="11" /><line x1="3" y1="17" x2="19" y2="17" /></svg></button>
      <div id="site-menu" class="header-menu" :class="{ 'header-menu--open': open }">
        <nav class="site-nav" :aria-label="messages.menu">
          <NuxtLink v-for="link in links" :key="link.path" :to="link.path" :class="{ 'site-nav--active': route.path === link.path || route.path.startsWith(link.path + '/') }">{{ compact && link.path === '/connect' ? messages.connectShort : link.label }}</NuxtLink>
        </nav>
        <div class="header-actions">
          <button type="button" class="language-button" @click="toggleLanguage">{{ messages.language }}</button>
          <NuxtLink v-if="session.status === 'signed-in'" class="account-link" to="/account"><span class="avatar" aria-hidden="true">{{ session.account?.email?.charAt(0).toUpperCase() }}</span>{{ messages.account }}</NuxtLink>
          <a v-else class="btn btn-secondary" :href="loginUrl(route.fullPath)">
            <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true"><path d="M17.6 9.2c0-.6-.1-1.2-.2-1.8H9v3.4h4.8a4.1 4.1 0 0 1-1.8 2.7v2.2h2.9c1.7-1.6 2.7-3.9 2.7-6.5z" fill="#4285F4" /><path d="M9 18c2.4 0 4.5-.8 6-2.2l-2.9-2.2c-.8.5-1.8.9-3.1.9-2.4 0-4.4-1.6-5.1-3.8H.9v2.3A9 9 0 0 0 9 18z" fill="#34A853" /><path d="M3.9 10.7a5.4 5.4 0 0 1 0-3.4V5H.9a9 9 0 0 0 0 8z" fill="#FBBC05" /><path d="M9 3.6c1.3 0 2.5.5 3.5 1.4l2.6-2.6A9 9 0 0 0 .9 5l3 2.3C4.6 5.2 6.6 3.6 9 3.6z" fill="#EA4335" /></svg>{{ messages.signIn }}
          </a>
        </div>
      </div>
    </div>
  </header>
</template>
