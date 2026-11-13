// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onMounted, ref } from 'vue'
import Account from '../app/pages/account.vue'

const report = { report_id: 1, kind: 'wrong_text', document_key: 'vn:vbpl:1', anchor: 'a1', document_number: '1/2020/QH', target: '', suggestion: { eff_from: '2020-01-01' }, state: 'open', created_at: '2026-01-01T00:00:00Z', changed_at: '2026-01-01T00:00:00Z' }
afterEach(() => vi.unstubAllGlobals())
it('appends pages, labels reports, and stops at the last page', async () => {
  setActivePinia(createPinia())
  const calls: string[] = []
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => ({ client: { GET: async (path: string, options?: { params: { query: { next: string } } }) => {
    if (path !== '/api/v1/me/reports') return { data: [] }
    calls.push(options?.params.query.next || '')
    return calls.length === 1 ? { data: { reports: [report], next: 'cursor' } } : { data: { reports: [{ ...report, report_id: 2, kind: 'future_kind', state: 'future_state' }] } }
  } } }))
  const wrapper = mount(Account)
  await flushPromises()
  expect(wrapper.text()).toContain('Wrong text')
  expect(wrapper.text()).toContain('Effective from')
  expect(wrapper.get('a.document-number').attributes('href')).toBe('/vn/documents/vn:vbpl:1#a1')
  await wrapper.get('button.reports-more').trigger('click')
  await flushPromises()
  expect(calls).toEqual(['', 'cursor'])
  expect(wrapper.text()).not.toContain('future_kind')
  expect(wrapper.text()).not.toContain('future_state')
  expect(wrapper.find('button.reports-more').exists()).toBe(false)
  expect(wrapper.text()).toContain('Wrong text')
})

it('keeps loaded reports and shows an error when the next page fails', async () => {
  setActivePinia(createPinia())
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => ({ client: { GET: async (path: string, options?: { params: { query: { next?: string } } }) => {
    if (path !== '/api/v1/me/reports') return { data: [] }
    if (options?.params.query.next) return { error: { error: { code: 'invalid_request' } } }
    return { data: { reports: [{ ...report, document_key: 'invalid/key', state: 'accepted' }], next: 'cursor' } }
  } } }))
  const wrapper = mount(Account)
  await flushPromises()
  expect(wrapper.find('a.document-number').exists()).toBe(false)
  await wrapper.get('button.reports-more').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Wrong text')
  expect(wrapper.get('[role=alert]').text()).toBe('The request is invalid.')
})

it.each([['en', 'In force'], ['vi', 'Còn hiệu lực']])('localizes a suggested status in %s', async (language, label) => {
  setActivePinia(createPinia())
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref(language))
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => ({ client: { GET: async (path: string) => path === '/api/v1/me/reports'
    ? { data: { reports: [{ ...report, suggestion: { status_class: 'in_force' } }] } } : { data: [] } } }))
  const wrapper = mount(Account)
  await flushPromises()
  expect(wrapper.text()).toContain(label)
  expect(wrapper.text()).not.toContain('in_force')
  wrapper.unmount()
})
