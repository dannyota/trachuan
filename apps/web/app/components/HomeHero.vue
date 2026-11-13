<script setup lang="ts">
import { loginUrl } from '../api/next'
import { useSearchDraftStore } from '../stores/search-draft'
import { useSessionStore } from '../stores/session'
import { homeMessages } from '../../i18n/home'
import type { Language } from '../../i18n'

const language = useState<Language>('language', () => 'vi')
const labels = computed(() => homeMessages(language.value))
const session = useSessionStore()
const draft = useSearchDraftStore()
const query = ref('')
const inForceOnly = ref(true)
const mounted = ref(false)
onMounted(() => { mounted.value = true })
const signedIn = computed(() => mounted.value && session.status === 'signed-in')
const suggestions = computed(() => [labels.value.suggestion1, labels.value.suggestion2, labels.value.suggestion3, labels.value.suggestion4])
async function submit() {
  if (!signedIn.value) {
    await navigateTo(loginUrl('/vn/search'), { external: true })
    return
  }
  if (!query.value.trim()) return
  draft.draft = { query: query.value, inForceOnly: inForceOnly.value }
  await navigateTo('/vn/search')
}
</script>
<template>
  <section class="home-hero">
    <div class="container home-hero-inner">
      <p class="home-eyebrow">{{ labels.eyebrow }}</p>
      <h1>{{ labels.title }}</h1>
      <p class="home-intro">{{ labels.intro }}</p>
      <form class="home-form" role="search" @submit.prevent="submit">
        <label for="home-query">{{ labels.query }}</label>
        <div class="home-query-row"><input id="home-query" v-model="query" class="input" type="search" autocomplete="off" :placeholder="labels.placeholder"><button class="btn btn-primary" type="submit"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>{{ labels.submit }}</button></div>
        <div class="home-form-options"><label><input v-model="inForceOnly" type="checkbox"> {{ labels.inForce }}</label><span v-if="!signedIn">{{ labels.signIn }}</span></div>
      </form>
      <div class="home-suggestions"><span>{{ labels.suggestions }}</span><button v-for="suggestion in suggestions" :key="suggestion" class="chip" type="button" @click="query = suggestion">{{ suggestion }}</button></div>
    </div>
  </section>
</template>
