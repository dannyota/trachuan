<script setup lang="ts">
import { shellMessages, type Language } from '../../i18n'
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => shellMessages(language.value))
</script>
<template>
  <footer class="site-footer"><div class="container footer-inner">
    <span>© 2026 Tra Chuẩn</span><NuxtLink to="/terms">{{ messages.terms }}</NuxtLink><NuxtLink to="/privacy">{{ messages.privacy }}</NuxtLink><a href="mailto:support@trachuan.vn">support@trachuan.vn</a><NuxtLink to="/sources">{{ messages.sources }}</NuxtLink><span class="footer-source">{{ messages.sourceNote }}</span>
  </div></footer>
</template>
