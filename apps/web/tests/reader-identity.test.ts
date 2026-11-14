// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
it('shows the release and status basis without inventing a status date', async () => {
  setupReader([{ data: { release: '20261111_1', item: { complete: true, document: { number: '12/2026/QH', title: 'Test law', status: { class: 'in_force', current: true, status_basis: 'source_status' }, text_source: 'source_tree', text_complete: true, source_url: 'https://vbpl.vn/test' }, units: [] } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).toContain('Release 11 Nov 2026 (build 1)')
  expect(wrapper.text()).toContain('according to vbpl.vn')
  expect(wrapper.text()).not.toContain('Status as of')
})
it('offers the search return link before the document stamp', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, document: { number: '1/2099', title: 'Test law', status: { class: 'in_force', current: true }, text_source: 'none', text_complete: false }, units: [] } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  const band = wrapper.get('.reader-identity')
  const link = wrapper.get('.reader-back-mobile a[href="/vn/search"]')
  expect(link.text()).toContain('Back to results')
  expect(link.element.compareDocumentPosition(band.get('.stamp').element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
it('sets the title from the document number or a truncated unnumbered title', async () => {
  const head = vi.fn()
  setupReader([{ data: { release: '20990101', item: { complete: true, document: { number: 'Không số', title: 'A'.repeat(70), status: { class: 'in_force' } }, units: [] } } }])
  vi.stubGlobal('useHead', head)
  const wrapper = mount(Reader)
  await flushPromises()
  expect(head.mock.calls.at(-1)?.[0]().title).toBe(`${'A'.repeat(60)} · Tra Chuẩn`)
  wrapper.unmount()
})
it('sets the title from a numbered document', async () => {
  const head = vi.fn()
  setupReader([{ data: { release: '20990101', item: { complete: true, document: { number: '1/2099', title: 'Synthetic title', status: { class: 'in_force' } }, units: [] } } }])
  vi.stubGlobal('useHead', head)
  const wrapper = mount(Reader)
  await flushPromises()
  expect(head.mock.calls.at(-1)?.[0]().title).toBe('1/2099 · Tra Chuẩn')
  wrapper.unmount()
})
