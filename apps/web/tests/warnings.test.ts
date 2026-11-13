// @vitest-environment happy-dom
import { computed, onUnmounted, ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import SearchResults from '../app/components/SearchResults.vue'
import Reader from '../app/pages/vn/documents/[key].vue'
import { warningsByTarget } from '../app/utils/reader-units'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
const warning = { dir: 'in', kind: 'repeals', key: 'vn:vbpl:2', number: '2/2024', in_release: true, source: 'vn:vbpl:2#dieu-1', basis: 'text', target: 'vn:vbpl:1#dieu-8', citation: 'Khoản 2 Điều 8', exact: true }
const link = { template: '<a :href="to"><slot /></a>', props: ['to'] }

it.each([
  ['en', ['repeals', 'replaces', 'suspends', 'amends or supplements']],
  ['vi', ['bãi bỏ', 'thay thế', 'ngưng hiệu lực', 'sửa đổi, bổ sung']],
])('renders each warning kind in %s on cards', (language, verbs) => {
  vi.stubGlobal('useState', () => ref(language))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onUnmounted', onUnmounted)
  const warnings = ['repeals', 'replaces', 'suspends', 'amends_supplements'].map(kind => ({ ...warning, kind }))
  const wrapper = mount(SearchResults, { props: { result: { release: '20990101', hits: [{ doc: 'vn:vbpl:1', handle: 'vn:vbpl:1#dieu-8', complete: true, warnings, warnings_omitted: 2 }], documents: { 'vn:vbpl:1': { status: { current: true }, text_source: 'source_tree', text_complete: true, related: [{ kind: 'cites', number: '3/2024', in_release: false }] } } } }, global: { stubs: { NuxtLink: link } } })
  const card = wrapper.find('.result-card')
  for (const verb of verbs) expect(card.text()).toContain(`2/2024 ${verb} Khoản 2 Điều 8`)
  expect(card.text()).toContain(language === 'en' ? 'and 2 more warnings' : 'và 2 cảnh báo khác')
  expect(card.text()).not.toContain('3/2024')
  expect(card.find('a[href="/vn/documents/vn:vbpl:2?release=20990101#dieu-1"]').exists()).toBe(true)
})

it('renders named paths and reference notes without guessing unknown shapes', () => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onUnmounted', onUnmounted)
  const warnings = [{ ...warning, exact: false, named: 'dieu-8/khoan-9/diem-a' }, { ...warning, exact: false, named: 'article-1/khoan-9' }, { ...warning, basis: 'vbpl_reference' }]
  const wrapper = mount(SearchResults, { props: { result: { hits: [{ complete: true, warnings }] } }, global: { stubs: { NuxtLink: link } } })
  expect(wrapper.text()).toContain('Điều 8, Khoản 9, Điểm a (this clause or point is not in the loaded text)')
  expect(wrapper.text()).not.toContain('article-1/khoan-9')
  expect(wrapper.text()).toContain('part of this document')
  expect(wrapper.text()).toContain('Khoản 2 Điều 8 (per a VBPL link)')
})

it('places warnings at their target and groups the rail in document order', async () => {
  const first = { ...warning, target: 'vn:vbpl:1#dieu-1', citation: 'Điều 1' }
  const second = { ...warning, target: 'vn:vbpl:1#dieu-2', citation: 'Điều 2' }
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [{ kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', warnings: [second, first] }, { kind: 'provision', anchor: 'dieu-2', label: 'Điều 2', warnings: [second, first] }] } } }])
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: link } } })
  await flushPromises()
  const articles = wrapper.findAll('.reader-content article')
  expect(articles[0]!.text()).toContain('2/2024 repeals Điều 1')
  expect(articles[0]!.text()).not.toContain('2/2024 repeals Điều 2')
  expect(articles[1]!.text()).toContain('2/2024 repeals Điều 2')
  const rail = wrapper.find('.reader-rail').text()
  expect(rail).toContain('Warnings')
  expect(rail.indexOf('2/2024 repeals Điều 1')).toBeLessThan(rail.indexOf('2/2024 repeals Điều 2'))
})

it('recovers an inherited warning displaced from a capped target and deduplicates it', async () => {
  const article = { ...warning, target: 'vn:vbpl:1#dieu-1', citation: 'Điều 1' }
  const capped = Array.from({ length: 10 }, (_, i) => ({ ...warning, target: `vn:vbpl:1#dieu-1/khoan-${i + 1}`, citation: `Khoản ${i + 1} Điều 1` }))
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    { kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', warnings: capped, warnings_omitted: 1 },
    { kind: 'provision', anchor: 'dieu-1/khoan-1', label: '1.', warnings: [capped[0], article] },
    { kind: 'provision', anchor: 'dieu-1/khoan-2', label: '2.', warnings: [capped[1], article] },
  ] } } }])
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: link } } })
  await flushPromises()
  const articles = wrapper.findAll('.reader-content article')
  expect(articles[0]!.text().match(/2\/2024 repeals Điều 1/g)).toHaveLength(1)
  expect(articles[1]!.text()).not.toContain('2/2024 repeals Điều 1')
  expect(wrapper.find('.reader-rail').text().match(/2\/2024 repeals Điều 1/g)).toHaveLength(1)
})

it.each([['en', 'and 3 more warnings'], ['vi', 'và 3 cảnh báo khác']])('shows omitted counts on the unit and rail in %s', async (language, notice) => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [{ kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', citation: 'Điều 1', warnings_omitted: 3 }] } } }])
  vi.stubGlobal('useState', () => ref(language))
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: link } } })
  await flushPromises()
  expect(wrapper.find('.reader-content article').text()).toContain(notice)
  expect(wrapper.find('.reader-rail').text()).toContain(notice)
})

it('uses citations to distinguish target groups with repeated labels', async () => {
  const first = { ...warning, target: 'vn:vbpl:1#dieu-1/khoan-1', citation: 'Khoản 1 Điều 1' }
  const second = { ...warning, target: 'vn:vbpl:1#dieu-2/khoan-1', citation: 'Khoản 1 Điều 2' }
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    { kind: 'provision', anchor: 'dieu-1/khoan-1', label: '1.', citation: 'Khoản 1 Điều 1', warnings: [first] },
    { kind: 'provision', anchor: 'dieu-2/khoan-1', label: '1.', citation: 'Khoản 1 Điều 2', warnings: [second] },
  ] } } }])
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: link } } })
  await flushPromises()
  expect(wrapper.findAll('.reader-rail .warnings h3').map(heading => heading.text())).toEqual(['Khoản 1 Điều 1', 'Khoản 1 Điều 2'])
})


it('renders both missing clauses at their shared target in the reader and rail', async () => {
  const first = { ...warning, citation: 'Điều 8', exact: false, named: 'dieu-8/khoan-9' }
  const second = { ...first, named: 'dieu-8/khoan-10' }
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    { kind: 'provision', anchor: 'dieu-8', label: 'Điều 8', warnings: [first, second] },
  ] } } }])
  const wrapper = mount(Reader, { global: { stubs: { NuxtLink: link } } })
  await flushPromises()
  for (const section of [wrapper.find('.reader-content article'), wrapper.find('.reader-rail .warnings')]) {
    expect(section.text()).toContain('2/2024 repeals Điều 8, khoản 9 (this clause or point is not in the loaded text)')
    expect(section.text()).toContain('2/2024 repeals Điều 8, khoản 10 (this clause or point is not in the loaded text)')
  }
})

it.each([
  ['exact', { ...warning, exact: false }],
  ['basis', { ...warning, basis: 'vbpl_reference' }],
])('keeps warnings distinct when %s differs', (_field, other) => {
  const groups = warningsByTarget([{ kind: 'provision', warnings: [warning, other, warning] }])
  expect(groups.get(warning.target)).toHaveLength(2)
})
