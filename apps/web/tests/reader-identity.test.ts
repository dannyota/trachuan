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
  const link = band.get('a[href="/vn/search"]')
  expect(link.text()).toContain('Back to results')
  expect(link.element.compareDocumentPosition(band.get('.stamp').element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
