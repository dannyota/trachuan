// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onMounted, ref } from 'vue'
import { createApi } from '../app/api/client'
import SearchNodePicker from '../app/components/SearchNodePicker.vue'

afterEach(() => vi.unstubAllGlobals())
it('loads nodes, matches locally, expands without sending lookup text, and limits selection', async () => {
  const calls: unknown[] = []
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  const api = createApi({ origin: 'http://localhost', fetch: async input => {
    const request = input as Request
    calls.push(request.url)
    return new Response(JSON.stringify({ nodes: calls.length === 1 ? [{ code: '1', name: 'Ngân hàng', children: Array.from({ length: 21 }, (_, i) => ({ code: `1.${i + 1}`, name: `Node ${i + 1}` })) }] : [{ code: '1.1.c1', name: 'Child' }], truncated: false }), { headers: { 'content-type': 'application/json' } })
  } })
  vi.stubGlobal('useApi', () => api)
  const wrapper = mount(SearchNodePicker, { props: { modelValue: [] } })
  await flushPromises()
  expect(calls[0]).toContain('depth=2')
  await wrapper.get('input[type="search"]').setValue('ngan hang')
  expect(wrapper.text()).toContain('Ngân hàng')
  await wrapper.get('input[type="search"]').setValue('node 1')
  await wrapper.findAll('button').find(button => button.attributes('aria-label') === 'Show children')!.trigger('click')
  await flushPromises()
  expect(JSON.stringify(calls)).not.toMatch(/ngan hang|node 1/)
  expect([location.href, JSON.stringify(history.state), JSON.stringify(localStorage), JSON.stringify(sessionStorage), document.title].join(' ')).not.toContain('ngan hang')
  expect(calls[1]).toContain('depth=1')
  await wrapper.get('input[type="search"]').setValue('')
  await wrapper.setProps({ modelValue: Array.from({ length: 20 }, (_, i) => `1.${i + 1}`) })
  expect(wrapper.findAll('fieldset > div button')).toHaveLength(20)
  expect(wrapper.findAll('input[type="checkbox"]').find(input => !(input.element as HTMLInputElement).checked)!.attributes('disabled')).toBeDefined()
})

it('matches descriptions and hides descendants when their ancestor collapses', async () => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => ({ client: { GET: async (_path: string, options: { params: { query: { node?: string } } }) => ({ data: { nodes: options.params.query.node ? [{ code: '1.1.c1', name: 'Child', description: 'Đất đai' }] : [{ code: '1', name: 'Root', children: [{ code: '1.1', name: 'Parent' }] }] } }) } }))
  const wrapper = mount(SearchNodePicker, { props: { modelValue: [] } })
  await flushPromises()
  await wrapper.findAll('button').find(button => button.attributes('aria-label') === 'Show children' && button.element.parentElement?.textContent?.includes('Parent'))!.trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Child')
  await wrapper.get('input[type="search"]').setValue('dat dai')
  expect(wrapper.text()).toContain('Child')
  await wrapper.get('input[type="search"]').setValue('')
  await wrapper.findAll('button').find(button => button.attributes('aria-label') === 'Hide children')!.trigger('click')
  expect(wrapper.text()).not.toContain('Child')
})

it('collapses and reopens root children consistently', async () => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => ({ client: { GET: async () => ({ data: { nodes: [{ code: '1', name: 'Root', children: [{ code: '1.1', name: 'Parent' }] }] } }) } }))
  const wrapper = mount(SearchNodePicker, { props: { modelValue: [] } })
  await flushPromises()
  const root = wrapper.findAll('li').find(li => li.text().includes('Root'))!
  expect(root.get('button').attributes('aria-label')).toBe('Hide children')
  expect(root.get('button').attributes('aria-expanded')).toBe('true')
  expect(wrapper.text()).toContain('Parent')
  await root.get('button').trigger('click')
  expect(root.get('button').attributes('aria-label')).toBe('Show children')
  expect(root.get('button').attributes('aria-expanded')).toBe('false')
  expect(wrapper.text()).not.toContain('Parent')
  await root.get('button').trigger('click')
  expect(wrapper.text()).toContain('Parent')
})

it('retries an initial taxonomy network failure without losing a selected node', async () => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  let attempts = 0
  const api = createApi({ origin: 'http://localhost', fetch: async () => {
    if (++attempts === 1) throw new Error('network')
    return new Response(JSON.stringify({ nodes: [{ code: '1', name: 'Root' }], truncated: false }), { headers: { 'content-type': 'application/json' } })
  } })
  vi.stubGlobal('useApi', () => api)
  const wrapper = mount(SearchNodePicker, { props: { modelValue: ['1'] } })
  await flushPromises()
  expect(wrapper.text()).toContain('Filters unavailable')
  expect(wrapper.text()).toContain('Topics ×')
  await wrapper.findAll('button').find(button => button.text() === 'Retry')!.trigger('click')
  await flushPromises()
  expect(wrapper.text()).not.toContain('Filters unavailable')
  expect(wrapper.text()).toContain('Root ×')
})

it('shows a message when no topics match', async () => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => ({ client: { GET: async () => ({ data: { nodes: [{ code: '1', name: 'Root' }] } }) } }))
  const wrapper = mount(SearchNodePicker, { props: { modelValue: [] } })
  await flushPromises()
  await wrapper.get('input[type="search"]').setValue('missing')
  expect(wrapper.text()).toContain('No matching topics.')
})
