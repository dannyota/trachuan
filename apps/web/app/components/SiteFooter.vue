<script setup lang="ts">
import { shellMessages, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => shellMessages(language.value))
</script>
<template>
  <footer class="site-footer"><div class="container footer-inner">
    <NuxtLink class="site-logo" to="/"><svg width="34" height="34" viewBox="0 0 32 32" aria-hidden="true"><circle cx="16" cy="16" r="14.5" fill="var(--color-seal-footer)" /><text x="16" y="21.5" text-anchor="middle" font-family="NotoSerif, Georgia, serif" font-size="15" font-weight="700" fill="var(--color-footer)">TC</text></svg><span>Tra Chuẩn</span></NuxtLink><span>© 2026 Tra Chuẩn</span><NuxtLink to="/terms">{{ messages.terms }}</NuxtLink><NuxtLink to="/privacy">{{ messages.privacy }}</NuxtLink><a href="mailto:support@trachuan.vn">support@trachuan.vn</a><NuxtLink to="/sources">{{ messages.sources }}</NuxtLink><span class="footer-source">{{ messages.sourceNote }}</span>
  </div></footer>
</template>
