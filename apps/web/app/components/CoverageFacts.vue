<script setup lang="ts">
import type { components } from '@trachuan/schema/gen/openapi'
import { messagesFor, type Language } from '../../i18n'

defineProps<{ status: components['schemas']['CorpusStatus'] | null }>()
const language = useState<Language>('language', () => 'vi')
const messages = computed(() => messagesFor(language.value))
</script>

<template>
  <section>
    <h2>{{ messages.coverage }}</h2>
    <p v-if="!status">{{ messages.coverageUnavailable }}</p>
    <dl v-else>
      <dt>{{ messages.release }}</dt><dd>{{ status.release }}</dd>
      <dt>{{ messages.documents }}</dt><dd>{{ status.documents }}</dd>
      <dt>{{ messages.provisions }}</dt><dd>{{ status.provisions }}</dd>
      <dt>{{ messages.nodes }}</dt><dd>{{ status.nodes }}</dd>
      <template v-if="status.newest_source_date"><dt>{{ messages.newestSource }}</dt><dd>{{ status.newest_source_date }}</dd></template>
      <dt>{{ messages.gaps }}</dt><dd><span v-for="(count, kind) in status.gaps" :key="kind">{{ kind }}: {{ count }} </span></dd>
    </dl>
  </section>
</template>
