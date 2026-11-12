<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'

type Node = components['schemas']['NodeEntry']
const props = defineProps<{ modelValue: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi()
const lookup = ref('')
const nodes = ref<Node[]>([])
const expanded = ref<Record<string, boolean>>({})
const loaded = ref<Record<string, boolean>>({})
const failed = ref(false)
function flatten(entries: Node[]): Node[] {
  return entries.flatMap(node => [node, ...flatten(node.children || [])])
}
function fold(value: string) {
  return value.normalize('NFC').toLowerCase().normalize('NFD').replace(/\p{M}/gu, '').replace(/đ/g, 'd')
}
const matches = computed(() => {
  const ordered: Node[] = []
  function visit(parent: string) {
    for (const node of nodes.value) {
      const code = node.code || ''
      if (code.split('.').slice(0, -1).join('.') !== parent || !code) continue
      ordered.push(node)
      if (lookup.value || code.split('.').length === 1 || expanded.value[code]) visit(code)
    }
  }
  visit('')
  const needle = fold(lookup.value)
  return ordered.filter(node => fold(`${node.name || ''} ${node.description || ''} ${node.code || ''}`).includes(needle))
})
function toggleSelection(code: string) {
  if (props.modelValue.includes(code)) emit('update:modelValue', props.modelValue.filter(item => item !== code))
  else if (props.modelValue.length < 20) emit('update:modelValue', [...props.modelValue, code])
}
async function expand(code: string) {
  if (expanded.value[code]) { expanded.value[code] = false; return }
  if (!loaded.value[code]) {
    try {
      const response = await api.client.GET('/api/v1/public/taxonomy', { params: { query: { node: code, depth: 1 } } })
      if (!response.data) throw new Error('unavailable')
      const existing = new Set(nodes.value.map(node => node.code))
      nodes.value.push(...flatten(response.data.nodes || []).filter(node => !existing.has(node.code)))
      loaded.value[code] = true
      failed.value = false
    } catch { failed.value = true; return }
  }
  expanded.value[code] = true
}
onMounted(async () => {
  try {
    const response = await api.client.GET('/api/v1/public/taxonomy', { params: { query: { depth: 2 } } })
    if (!response.data) throw new Error('unavailable')
    nodes.value = flatten(response.data.nodes || [])
  } catch { failed.value = true }
})
</script>
<template>
  <fieldset>
    <legend>{{ messages.searchNodes }}</legend>
    <label>{{ messages.searchNodeLookup }} <input v-model="lookup" type="search" autocomplete="off"></label>
    <p v-if="failed" role="status">{{ messages.errors.unavailable }}</p>
    <div><button v-for="code in modelValue" :key="code" type="button" @click="toggleSelection(code)">{{ nodes.find(node => node.code === code)?.name || code }} ×</button></div>
    <ul><li v-for="node in matches" :key="node.code">
      <label><input type="checkbox" :checked="modelValue.includes(node.code || '')" :disabled="!modelValue.includes(node.code || '') && modelValue.length >= 20" @change="node.code && toggleSelection(node.code)"> {{ node.name }}</label>
      <button v-if="node.code" type="button" :aria-expanded="!!expanded[node.code]" @click="expand(node.code)">{{ expanded[node.code] ? messages.topicCollapse : messages.topicExpand }}</button>
    </li></ul>
  </fieldset>
</template>
