// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
it('renders legal text literally', async () => {
  setupReader([{ data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', anchor: 'a', text: '<Điều 1> & text' }] } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).toContain('<Điều 1> & text')
  expect(wrapper.find('điều').exists()).toBe(false)
})
it('indents lettered points while preserving the legal text', async () => {
  setupReader([{ data: { release: '20990101_1', item: { complete: true, units: [{ kind: 'provision', anchor: 'dieu-1', text: '1. Clause\na) Point text' }] } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  const point = wrapper.findAll('.law-text').find(element => element.text() === 'a) Point text')
  expect(point?.classes()).toContain('reader-point')
})

it('wraps the target clause and its points without wrapping the next clause', async () => {
  const fixture = (await import('./fixtures/reader-171687.json')).default[0]!
  setupReader([{ data: fixture }])
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:171687' }, query: {}, hash: '#dieu-11/khoan-1' }))
  const wrapper = mount(Reader)
  await flushPromises()
  const band = wrapper.get('.reader-unit-target')
  expect(band.find('[data-anchor="dieu-11/khoan-1"]').exists()).toBe(true)
  expect(band.find('[data-anchor="dieu-11/khoan-1/diem-a"]').exists()).toBe(true)
  expect(band.find('[data-anchor="dieu-11/khoan-1/diem-b"]').exists()).toBe(true)
  expect(band.find('[data-anchor="dieu-11/khoan-2"]').exists()).toBe(false)
})
it('renders source labels once for the real provisions', async () => {
  const fixture = (await import('./fixtures/reader-171687.json')).default[0]!
  setupReader([{ data: fixture }])
  const wrapper = mount(Reader)
  await flushPromises()
  const article = wrapper.get('[data-anchor="dieu-11"]')
  expect(article.findAll('h2')).toHaveLength(1)
  expect(article.get('h2').text()).toContain('Các hình thức xác nhận')
  expect(article.findAll('.law-text')).toHaveLength(0)
  expect(wrapper.get('[data-anchor="dieu-11/khoan-1"]').find('h2').exists()).toBe(false)
  expect(wrapper.get('[data-anchor="dieu-11/khoan-1/diem-a"]').find('h2').exists()).toBe(false)
  expect(wrapper.get('[data-anchor="chuong-i"]').findAll('.law-text')).toHaveLength(0)
})
it.each([['dieu-1', 'Article you are viewing'], ['dieu-1/khoan-1', 'Clause you are viewing'], ['dieu-1/khoan-1/diem-a', 'Point you are viewing']])('names the target level for %s', async (anchor, label) => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [{ kind: 'provision', anchor, label: 'Điều 1', text: 'Text' }] } } }])
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query: {}, hash: `#${anchor}` }))
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.get('.reader-target-label').text()).toContain(label)
  wrapper.unmount()
})
