<script setup lang="ts">
import { watchEffect } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'

type Node = components['schemas']['NodeEntry']
const props = defineProps<{ modelValue: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]], labels: [value: Record<string, string>] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const api = useApi({ localUnavailable: true })
const lookup = ref('')
const nodes = ref<Node[]>([])
const expanded = ref<Record<string, boolean>>({})
const loaded = ref<Record<string, boolean>>({})
const failed = ref(false)
const failedNode = ref('')
watchEffect(() => emit('labels', Object.fromEntries(nodes.value.filter(node => node.code && node.name).map(node => [node.code!, node.name!]))))
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
      if (lookup.value || expanded.value[code]) visit(code)
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
    } catch { failed.value = true; failedNode.value = code; return }
  }
  expanded.value[code] = true
}
async function loadRoots() {
  try {
    const response = await api.client.GET('/api/v1/public/taxonomy', { params: { query: { depth: 2 } } })
    if (!response.data) throw new Error('unavailable')
    const roots = response.data.nodes || []
    nodes.value = flatten(roots)
    for (const node of roots) if (node.code) {
      expanded.value[node.code] = true
      loaded.value[node.code] = true
    }
    failed.value = false
  } catch { failed.value = true; failedNode.value = '' }
}
function retry() {
  if (failedNode.value) return expand(failedNode.value)
  return loadRoots()
}
onMounted(loadRoots)
</script>
<template>
  <fieldset class="node-picker">
    <legend>{{ messages.searchNodes }}</legend>
    <label>{{ messages.searchNodeLookup }} <input v-model="lookup" class="input" type="search" autocomplete="off"></label>
    <p v-if="failed" role="status">{{ messages.filtersUnavailable }} <button type="button" @click="retry">{{ messages.retry }}</button></p>
    <div class="node-selected"><button v-for="code in modelValue" :key="code" class="chip" type="button" @click="toggleSelection(code)">{{ nodes.find(node => node.code === code)?.name || messages.searchNodes }} ×</button></div>
    <ul><li v-for="node in matches" :key="node.code">
      <label><input type="checkbox" :checked="modelValue.includes(node.code || '')" :disabled="!modelValue.includes(node.code || '') && modelValue.length >= 20" @change="node.code && toggleSelection(node.code)"> {{ node.name }}</label>
      <button v-if="node.code" type="button" :aria-expanded="!!expanded[node.code]" @click="expand(node.code)">{{ expanded[node.code] ? messages.topicCollapse : messages.topicExpand }}</button>
    </li></ul>
  </fieldset>
</template>
