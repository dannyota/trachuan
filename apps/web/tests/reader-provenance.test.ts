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
  expect(wrapper.text()).toContain('11 Nov 2026 (build 1)')
  expect(wrapper.text()).toContain('abc123')
  expect(wrapper.find(`a[href="${source_url}"]`).exists()).toBe(source_url.startsWith('https:'))
})

it('shows indexed document nodes when opened directly', async () => {
  setupReader([{ data: { release: '20261111_1', nodes: { '1': { name: 'Ngân hàng' } }, item: { complete: true, document: { nodes: ['1', '2'], status: { current: true }, text_source: 'source_tree', text_complete: true }, units: [] } } }])
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  await flushPromises()
  expect(wrapper.text()).toContain('Ngân hàng')
  expect(wrapper.findAll('.reader-chips a')).toHaveLength(1)
})

it('shows ordered files with safe links, metadata and full hash copies', async () => {
  const hash = 'a'.repeat(64)
  const files = [
    { name: 'main.pdf', url: 'https://example.test/main', hash: 'b'.repeat(64), size: 1500, role: 'main', fetched_at: '2099-01-01T00:00:00Z' },
    { name: 'appendix.pdf', url: 'http://example.test/a', hash: 'c'.repeat(64), size: 2000000, role: 'appendix', fetched_at: null },
    { name: 'scan.pdf', url: null, hash: 'd'.repeat(64), size: 0, role: 'scan', fetched_at: null },
    { name: 'other.pdf', url: 'javascript:alert(1)', hash: 'e'.repeat(64), size: 1000, role: 'unknown', fetched_at: null },
  ]
  setupReader([{ data: { release: '20990101', fetch_sha256: hash, files, item: { complete: true, document: { status: { current: true }, text_source: 'none', text_complete: false, source_url: 'https://example.test/source' }, units: [] } } }])
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  const wrapper = mount(Reader)
  await flushPromises()
  const list = wrapper.get('ul.reader-files')
  expect(list.attributes('role')).toBe('list')
  const rows = list.findAll('li.reader-file')
  expect(rows).toHaveLength(4)
  expect(rows.map(row => row.find('.reader-file-name').text())).toEqual(files.map(file => file.name))
  expect(rows[0]!.find('a').attributes('href')).toBe('https://example.test/main')
  for (const row of rows.slice(1)) expect(row.find('a').exists()).toBe(false)
  expect(rows[0]!.text()).toContain('1.5 kB')
  expect(rows[0]!.text()).toContain('01 Jan 2099')
  expect(rows[1]!.text()).toContain('Appendix')
  expect(rows[1]!.text()).toContain('2 MB')
  expect(rows[2]!.text()).toContain('Scan')
  expect(rows[3]!.text()).toContain('Unknown')
  expect(rows[0]!.text()).toContain('bbbbbbbbbbbb…')
  expect(wrapper.find('.reader-technical a[href="https://example.test/source"]').exists()).toBe(false)
  expect(rows[0]!.get('button').text()).toBe('Copy full SHA-256')
  await wrapper.get('.reader-technical > p button').trigger('click')
  expect(writeText).toHaveBeenLastCalledWith(hash)
  for (const [index, row] of rows.entries()) {
    await row.get('button').trigger('click')
    expect(writeText).toHaveBeenLastCalledWith(files[index]!.hash)
  }
  expect(writeText).toHaveBeenCalledTimes(5)
})
