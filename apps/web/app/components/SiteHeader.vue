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
  if (route.path === '/link') language.value = next
  else location.reload()
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
          <a v-else-if="!['/login', '/signup', '/reset'].includes(route.path)" class="btn btn-secondary" :href="loginUrl(route.fullPath)">
            {{ messages.signIn }}
          </a>
        </div>
      </div>
    </div>
  </header>
</template>
