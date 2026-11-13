// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import TopicTree from '../app/components/TopicTree.vue'

afterEach(() => vi.unstubAllGlobals())

it('loads and displays children when a topic expands', async () => {
  const get = vi.fn().mockResolvedValue({ data: { nodes: [{ code: '1.1', name: 'Child', provisions: 4, documents: 1 }], truncated: false } })
  vi.stubGlobal('useApi', () => ({ client: { GET: get } }))
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  const wrapper = mount(TopicTree, { props: { nodes: [{ code: '1', name: 'First', provisions: 11, documents: 2 }] }, global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  await wrapper.get('button').trigger('click')
  await vi.waitFor(() => expect(wrapper.text()).toContain('Child'))
  expect(get).toHaveBeenCalledWith('/api/v1/public/taxonomy', { params: { query: { node: '1', depth: 1 } } })
})

it('opens topic pages with a server navigation', () => {
  vi.stubGlobal('useApi', () => ({ client: {} }))
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  const link = { props: { to: String, external: Boolean }, template: '<a><slot /></a>' }
  const wrapper = mount(TopicTree, { props: { nodes: [{ code: '1', name: 'First' }] }, global: { stubs: { NuxtLink: link } } })
  expect(wrapper.findComponent(link).props('external')).toBe(true)
})

it('formats topic counts for the selected language', () => {
  vi.stubGlobal('useApi', () => ({ client: {} }))
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  const wrapper = mount(TopicTree, { props: { nodes: [{ code: '1', name: 'First', provisions: 1200, documents: 25 }] }, global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  expect(wrapper.text()).toContain('1.200 quy định · 25 văn bản')
})
