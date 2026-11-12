// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
it.each(['javascript:alert(1)', 'http://example.test', 'https://example.test/doc'])('shows provenance with safe links for %s', async (source_url) => {
  setupReader([{ data: { release: '20261111_1', fetch_sha256: 'abc123', item: { complete: true, document: { status: { class: 'in_force', current: true }, text_source: 'source_tree', text_complete: true, source_url }, units: [] } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).toContain('20261111_1')
  expect(wrapper.text()).toContain('abc123')
  expect(wrapper.find(`a[href="${source_url}"]`).exists()).toBe(source_url.startsWith('https:'))
})

it('shows indexed document nodes when opened directly', async () => {
  setupReader([{ data: { release: '20261111_1', nodes: { '1': { name: 'Ngân hàng' } }, item: { complete: true, document: { nodes: ['1', '2'], status: { current: true }, text_source: 'source_tree', text_complete: true }, units: [] } } }])
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  await flushPromises()
  expect(wrapper.text()).toContain('Ngân hàng')
  expect(wrapper.findAll('a')).toHaveLength(1)
})
