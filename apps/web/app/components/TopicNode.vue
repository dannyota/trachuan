<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, statusMessage, type Language } from '../../i18n'
defineProps<{ page: components['schemas']['NodePage'] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
</script>
<template>
  <div>
    <nav><NuxtLink external to="/vn/topics">{{ messages.topics }}</NuxtLink><span v-for="item in page.path" :key="item.code"> / <NuxtLink external :to="`/vn/topics/${item.code}`">{{ item.name }}</NuxtLink></span></nav>
    <h1>{{ page.node?.name }}</h1>
    <p v-if="page.node?.description">{{ page.node.description }}</p>
    <NuxtLink :to="{ path: '/vn/search', query: { node: page.node?.code } }">{{ messages.topicSearch }}</NuxtLink>
    <h2>{{ messages.topicChildren }}</h2><TopicTree :nodes="page.node?.children || []" />
    <h2>{{ messages.documents }}</h2>
    <ul><li v-for="document in page.documents" :key="document.key"><NuxtLink :to="`/vn/documents/${document.key}`">{{ document.number }} {{ document.title }}</NuxtLink><p>{{ messages.topicType }}: {{ document.type }}; {{ messages.topicStatus }}: {{ statusMessage(language, document.status_class) }}</p></li></ul>
  </div>
</template>
