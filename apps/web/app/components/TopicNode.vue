<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
defineProps<{ page: components['schemas']['NodePage'] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
</script>
<template>
  <div class="topic-page">
    <nav :aria-label="messages.topics"><NuxtLink external to="/vn/topics">{{ messages.topics }}</NuxtLink><span v-for="item in page.path" :key="item.code"> / <NuxtLink external :to="`/vn/topics/${item.code}`">{{ item.name }}</NuxtLink></span></nav>
    <header class="page-intro"><h1>{{ page.node?.name }}</h1><p v-if="page.node?.description">{{ page.node.description }}</p></header>
    <NuxtLink class="btn btn-primary topic-search" :to="{ path: '/vn/search', query: { node: page.node?.code } }">{{ messages.topicSearch }}</NuxtLink>
    <section class="page-section"><h2>{{ messages.topicChildren }}</h2><TopicTree :nodes="page.node?.children || []" /></section>
    <section class="page-section"><h2>{{ messages.documents }}</h2><ul class="topic-documents"><li v-for="document in page.documents" :key="document.key"><ValidityStamp :status="document.status_class" /><span class="num">{{ document.number }}</span><NuxtLink :to="`/vn/documents/${document.key}`">{{ document.title }}</NuxtLink><span>{{ document.type }}</span></li></ul></section>
  </div>
</template>
