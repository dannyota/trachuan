// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, onMounted, ref, Suspense } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Home from '../app/pages/index.vue'
import sample from '../app/data/sample-result.json'

beforeEach(() => {
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
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

it('describes the actual sample citation and omits an unavailable source', async () => {
  const wrapper = await render()
  expect(wrapper.get('.home-sample ul').text()).toContain('Khoản 1 Điều 25 Thông tư 09/2020/TT-NHNN')
  expect(wrapper.get('.home-sample ul').text()).not.toContain('vbpl.vn')
})

it('describes the source only when the sample has an https URL', async () => {
  const key = sample.hits[0]!.doc as keyof typeof sample.documents
  const doc = sample.documents[key] as { source_url?: string }
  try {
    doc.source_url = 'http://example.invalid/source'
    expect((await render()).get('.home-sample ul').text()).not.toContain('vbpl.vn')
    doc.source_url = 'https://example.invalid/source'
    expect((await render()).get('.home-sample ul').text()).toContain('vbpl.vn')
  } finally { delete doc.source_url }
})
