<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { formatNumber } from '../content'
import { messagesFor, type Language } from '../../i18n'
import { searchMessages } from '../../i18n/search'
import SearchNodePicker from './SearchNodePicker.vue'

defineProps<{ inForce: boolean, asOf: string, nodes: string[], docTypes: string[], issuers: string[], facets: components['schemas']['CorpusStatus'] | null, failed: boolean }>()
const emit = defineEmits<{ 'update:inForce': [value: boolean], 'update:asOf': [value: string], 'update:nodes': [value: string[]], 'update:docTypes': [value: string[]], 'update:issuers': [value: string[]], retry: [], labels: [value: Record<string, string>] }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
const labels = computed(() => searchMessages(language.value))
const allTypes = ref(false)
const allIssuers = ref(false)
function toggle(list: string[], value: string, group: 'docTypes' | 'issuers') {
  const next = list.includes(value) ? list.filter(item => item !== value) : list.length < 10 ? [...list, value] : list
  if (group === 'docTypes') emit('update:docTypes', next)
  else emit('update:issuers', next)
}
</script>

<template>
  <div class="search-filters">
    <h2>{{ labels.filters }}</h2>
    <fieldset><legend>{{ labels.validity }}</legend>
      <label><input type="checkbox" :checked="inForce" @change="emit('update:inForce', ($event.target as HTMLInputElement).checked)"> {{ messages.searchInForce }}</label>
      <label>{{ messages.searchDate }} <span>{{ labels.dateHint }}</span> <input class="input" type="date" :value="asOf" @input="emit('update:asOf', ($event.target as HTMLInputElement).value)"></label>
    </fieldset>
    <p v-if="failed" role="status">{{ messages.filtersUnavailable }} <button type="button" @click="emit('retry')">{{ messages.retry }}</button></p>
    <fieldset v-for="group in ['docTypes', 'issuers'] as const" :key="group">
      <legend>{{ group === 'docTypes' ? messages.searchTypes : messages.searchIssuers }}</legend>
      <label v-for="facet in (group === 'docTypes' ? facets?.doc_types : facets?.issuers)?.slice(0, (group === 'docTypes' ? allTypes : allIssuers) ? undefined : 6)" :key="facet.value" class="facet-option">
        <input type="checkbox" :checked="(group === 'docTypes' ? docTypes : issuers).includes(facet.value || '')" :disabled="!(group === 'docTypes' ? docTypes : issuers).includes(facet.value || '') && (group === 'docTypes' ? docTypes : issuers).length >= 10" @change="toggle(group === 'docTypes' ? docTypes : issuers, facet.value || '', group)">
        <span>{{ facet.value }}</span><span class="facet-count">{{ formatNumber(facet.count, language) }}</span>
      </label>
      <button v-if="(group === 'docTypes' ? facets?.doc_types : facets?.issuers)?.length && (group === 'docTypes' ? facets!.doc_types!.length : facets!.issuers!.length) > 6" type="button" class="filter-more" @click="group === 'docTypes' ? allTypes = !allTypes : allIssuers = !allIssuers">{{ (group === 'docTypes' ? allTypes : allIssuers) ? labels.showLess : labels.showAll }}</button>
    </fieldset>
    <SearchNodePicker :model-value="nodes" @update:model-value="emit('update:nodes', $event)" @labels="emit('labels', $event)" />
  </div>
</template>
