<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'
import { pagesMessages } from '../../i18n/pages'
import { formatNumber } from '../content'

type Node = components['schemas']['NodeEntry']
defineProps<{ nodes: Node[] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => pagesMessages(language.value))
const api = useApi()
const children = ref<Record<string, Node[]>>({})
const expanded = ref<Record<string, boolean>>({})
const failed = ref<Record<string, boolean>>({})
async function toggle(code: string) {
  if (expanded.value[code]) { expanded.value[code] = false; return }
  if (!children.value[code]) {
    try {
      const result = await api.client.GET('/api/v1/public/taxonomy', { params: { query: { node: code, depth: 1 } } })
      if (!result.data) throw new Error('unavailable')
      children.value[code] = result.data.nodes ?? []
      failed.value[code] = false
    } catch { failed.value[code] = true; return }
  }
  expanded.value[code] = true
}
</script>
<template>
  <ul class="topic-tree">
    <li v-for="node in nodes" :key="node.code">
      <NuxtLink external :to="`/vn/topics/${node.code}`">{{ node.name }}</NuxtLink>
      <p v-if="node.description">{{ node.description }}</p>
      <p class="topic-counts">{{ labels.topicCounts(formatNumber(node.provisions ?? 0, language), formatNumber(node.documents ?? 0, language)) }}</p>
      <button v-if="node.code" type="button" :aria-expanded="!!expanded[node.code]" :aria-label="`${expanded[node.code] ? messages.topicCollapse : messages.topicExpand}: ${node.name}`" @click="toggle(node.code)">{{ expanded[node.code] ? messages.topicCollapse : messages.topicExpand }}</button>
      <p v-if="node.code && failed[node.code]" role="status">{{ messages.errors.unavailable }}</p>
      <TopicTree v-if="node.code && expanded[node.code]" :nodes="children[node.code] || []" />
    </li>
  </ul>
</template>
