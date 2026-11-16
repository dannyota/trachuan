// @vitest-environment happy-dom
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, onMounted, ref, Suspense } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Home from '../app/pages/index.vue'
import sample from '../app/data/sample-result.json'

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useHead', () => {})
  vi.stubGlobal('useRoute', () => ({ query: {} }))
  vi.stubGlobal('useApi', () => ({}))
  vi.stubGlobal('useAsyncData', async () => ({ data: ref({ status: null }) }))
})
afterEach(() => vi.unstubAllGlobals())

async function render() {
  const wrapper = mount(defineComponent({ render: () => h(Suspense, null, { default: () => h(Home) }) }), { global: { stubs: { HomeHero: true, HomeConnect: true, CoverageFacts: true, ResultCard: true, NuxtLink: true } } })
  await flushPromises()
  return wrapper
}

it('describes the actual sample citation and its source', async () => {
  const wrapper = await render()
  expect(wrapper.get('.home-explanation ul').text()).toContain('Điều 19 Luật 91/2025/QH15')
  expect(wrapper.get('.home-explanation ul').text()).toContain('Liên kết về bản gốc, khi nguồn có công bố.')
})

it('describes the source only when the sample has an https URL', async () => {
  const key = sample.hits[0]!.doc as keyof typeof sample.documents
  const doc = sample.documents[key] as { source_url?: string }
  const original = doc.source_url
  try {
    doc.source_url = 'http://example.invalid/source'
    expect((await render()).get('.home-explanation ul').text()).not.toContain('Liên kết về bản gốc, khi nguồn có công bố.')
    doc.source_url = 'https://example.invalid/source'
    expect((await render()).get('.home-explanation ul').text()).toContain('Liên kết về bản gốc, khi nguồn có công bố.')
  } finally {
    if (original === undefined) delete doc.source_url
    else doc.source_url = original
  }
})
