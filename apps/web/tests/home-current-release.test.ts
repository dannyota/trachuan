// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, onMounted, onUnmounted, reactive, ref, Suspense } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Home from '../app/pages/index.vue'
import ResultCard from '../app/components/ResultCard.vue'

const session = reactive<{ status: 'unknown' | 'signed-in' | 'signed-out' }>({ status: 'unknown' })
const get = vi.fn()
beforeEach(() => {
  session.status = 'unknown'
  get.mockReset()
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onUnmounted', onUnmounted)
  vi.stubGlobal('useRoute', () => ({ query: {} }))
  vi.stubGlobal('useApi', () => ({ session, client: { GET: get } }))
  vi.stubGlobal('useAsyncData', async () => ({ data: ref({ status: null }) }))
})
afterEach(() => vi.unstubAllGlobals())
async function render() {
  const wrapper = mount(defineComponent({ render: () => h(Suspense, null, { default: () => h(Home) }) }), { attachTo: document.body, global: { components: { ResultCard }, stubs: { HomeHero: true, HomeConnect: true, CoverageFacts: true, NuxtLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } } })
  await flushPromises()
  return wrapper
}

it('dates the sample status from its release', async () => {
  const wrapper = await render()
  expect(wrapper.get('.result-card-head').text()).toContain('Trạng thái tính đến 11/11/2026, theo nguồn')
  wrapper.unmount()
})

it('keeps the reader link without checking the current release for signed-in visitors', async () => {
  session.status = 'signed-in'
  const wrapper = await render()
  expect(get).not.toHaveBeenCalled()
  expect(wrapper.get('.result-card-footer > a').isVisible()).toBe(true)
  wrapper.unmount()
})
