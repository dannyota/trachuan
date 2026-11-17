// @vitest-environment happy-dom
import { computed, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import SearchResults from '../app/components/SearchResults.vue'
import ResultCard from '../app/components/ResultCard.vue'

beforeEach(() => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
})
afterEach(() => vi.unstubAllGlobals())

it('shows safe gap categories without exposing raw codes', () => {
  const wrapper = mount(SearchResults, { props: { result: { release: 'r1', abstain: false, hits: [{ doc: 'vn:vbpl:1', citation: 'Điều 1', complete: true, gaps: [{ kind: 'validity_conflict', message: 'not used' }, { kind: 'missing_effective_date' }, { kind: 'unrecognized_code' }, { kind: 'missing_effective_date' }, { kind: 'consolidated_status_unordered' }, { kind: 'consolidated_status_unresolved' }, { kind: 'ambiguous_tree_text' }, { kind: '__proto__' }] }] } }, global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  const card = wrapper.find('.result-card')
  expect(card.text()).toContain('Validity incomplete')
  expect(card.text()).toContain('Structure uncertain')
  expect(card.text()).toContain('Other')
  expect(card.text()).not.toContain('unrecognized_code')
  expect(card.text()).not.toContain('__proto__')
  expect(card.text()).not.toContain('not used')
})

it('identifies laws with identical citations and omits invalid links', () => {
  const doc = { status: { current: true }, text_source: 'source_tree' as const, text_complete: true }
  const wrapper = mount(SearchResults, { props: { result: { release: 'r1', abstain: false, documents: { 'vn:vbpl:one': { ...doc, number: '11/2024', title: 'First law' }, 'vn:vbpl:two': { ...doc, number: '22/2024', title: 'Second law' } }, hits: [{ doc: 'vn:vbpl:one', citation: 'Điều 1', complete: true }, { doc: 'vn:vbpl:two', citation: 'Điều 1', complete: true }, { doc: '../bad', citation: 'Điều 1', complete: true }] } }, global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  const cards = wrapper.findAll('.result-card')
  expect(cards[0]!.text()).toContain('11/2024')
  expect(cards[0]!.text()).toContain('First law')
  expect(cards[0]!.find('.num').text()).toBe('11/2024')
  expect(cards[1]!.text()).toContain('22/2024')
  expect(cards[1]!.text()).toContain('Second law')
  expect(cards[2]!.text()).not.toContain('../bad')
  expect(cards[2]!.find('a').exists()).toBe(false)
})

it('groups filtered hits and names only indexed nodes', () => {
  const wrapper = mount(SearchResults, { props: { result: { abstain: false, outside_filter_count: 9, nodes: { '1': { name: 'Ngân hàng' } }, hits: [{ citation: 'outside', complete: true, in_filter: false }, { citation: 'inside', complete: true, in_filter: true, nodes: ['1', '2'] }] } }, global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  expect(wrapper.text().indexOf('inside')).toBeLessThan(wrapper.text().indexOf('outside'))
  expect(wrapper.text()).toContain('9')
  expect(wrapper.text()).toContain('Ngân hàng')
  expect(wrapper.findAll('.result-nodes a')).toHaveLength(1)
})

it('shows matched fragments, highlighting, warnings, and safe document actions', async () => {
  const fragment = (anchor: string) => ({ identity: { document_key: 'vn:vbpl:1', anchor }, role: 'body', start: 0, end: 1, text_start: 0, text_end: 1, complete: true })
  const hit = { doc: 'vn:vbpl:1', handle: 'vn:vbpl:1#dieu-1', citation: 'Điều 1', complete: false, path: [{ heading: 'Heading' }], text: 'Một cụm từ phù hợp và nội dung.', fragments: [fragment('dieu-1/khoan-1'), fragment('dieu-1/khoan-2')], warnings: [{ number: '2/2099', kind: 'repeals', exact: true, citation: 'Điều 1', in_release: true, key: 'vn:vbpl:2' }] }
  const result = { release: '20990101', abstain: false, hits: [hit], documents: { 'vn:vbpl:1': { number: '1/2099', type: 'Luật', title: 'Test law', status: { class: 'in_force', current: true }, text_source: 'source_tree' as const, text_complete: true, source_url: 'https://vbpl.vn/test' } } }
  vi.stubGlobal('onUnmounted', () => {})
  const link = { template: '<a :href="to"><slot /></a>', props: ['to'] }
  const wrapper = mount(SearchResults, { props: { result, query: 'cụm từ phù hợp' }, global: { stubs: { NuxtLink: link } } })
  expect(wrapper.find('h2').text()).toBe('Điều 1. Heading')
  expect(wrapper.find('mark').text()).toBe('cụm từ phù hợp')
  expect(wrapper.find('a[href$="#dieu-1/khoan-1"]').text()).toContain('Khoản 1')
  expect(wrapper.find('a[href$="#dieu-1/khoan-2"]').text()).toContain('Khoản 2')
  expect(wrapper.text()).toContain('Check the repealing act before citing.')
  expect(wrapper.get('a[href="https://vbpl.vn/test"]').attributes('rel')).toBe('noopener noreferrer')
})

it('keeps non-current results in a separate details group on a low-confidence response', () => {
  const wrapper = mount(SearchResults, { props: { result: { abstain: true, reason: 'low_confidence', hits: [{ citation: 'Current', complete: true }, { citation: 'Outside', complete: true, in_filter: false }], non_current: [{ citation: 'Past', complete: true }] } }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.text()).toContain('Weak evidence')
  expect(wrapper.text().indexOf('Current')).toBeLessThan(wrapper.text().indexOf('Outside'))
  expect(wrapper.get('details').text()).toContain('Past')
  expect(wrapper.get('details').attributes('open')).toBeUndefined()
})

it('links distinct clause labels to their own anchors despite nested and repeated fragments', () => {
  const fragment = (anchor: string) => ({ identity: { document_key: 'vn:vbpl:1', anchor }, role: 'body', start: 0, end: 1, text_start: 0, text_end: 1, complete: true })
  const result = { release: '20990101', abstain: false, hits: [{ doc: 'vn:vbpl:1', handle: 'vn:vbpl:1#dieu-1', citation: 'Điều 1', complete: false, fragments: [fragment('dieu-1/khoan-1'), fragment('dieu-1/khoan-1/diem-a'), fragment('dieu-1/khoan-1'), fragment('dieu-1/khoan-2')] }] }
  const link = { template: '<a :href="to"><slot /></a>', props: ['to'] }
  const wrapper = mount(SearchResults, { props: { result }, global: { stubs: { NuxtLink: link } } })
  const links = wrapper.findAll('.result-members a')
  expect(links.map(link => link.text().replace(/^,\s*/, ''))).toEqual(['Điều 1, Khoản 1', 'Điều 1, Khoản 2'])
  expect(links.map(link => link.attributes('href'))).toEqual(['/vn/documents/vn:vbpl:1?release=20990101#dieu-1/khoan-1', '/vn/documents/vn:vbpl:1?release=20990101#dieu-1/khoan-2'])
})

it('counts explicit and unrecognised unknown validity values together', () => {
  const doc = (status: string) => ({ status: { class: status, current: false }, text_source: 'source_tree' as const, text_complete: true })
  const wrapper = mount(SearchResults, { props: { result: { abstain: false, documents: { a: doc('unknown'), b: doc('future'), c: doc('in_force') }, hits: [{ doc: 'a', complete: true }, { doc: 'b', complete: true }, { doc: 'c', complete: true }] } }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('.search-legend').text()).toContain('Unknown 2')
  expect(wrapper.get('.search-legend').text()).toContain('In force 1')
})

it('offers expansion only when the rendered text overflows four lines', async () => {
  vi.stubGlobal('onUnmounted', () => {})
  const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(80)
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(80)
  const wrapper = mount(SearchResults, { props: { result: { abstain: false, hits: [{ text: 'Short text', complete: true }] } }, global: { stubs: { NuxtLink: true } } })
  await wrapper.vm.$nextTick()
  expect(wrapper.find('.result-toggle').exists()).toBe(false)
  scroll.mockReturnValue(160)
  await wrapper.setProps({ result: { abstain: false, hits: [{ text: 'Longer text', complete: true }] } })
  await flushPromises()
  expect(wrapper.get('.result-toggle').attributes('aria-expanded')).toBe('false')
  expect(wrapper.get('.result-toggle svg path').attributes('d')).toBe('m3 6 5 5 5-5')
  await wrapper.get('.result-toggle').trigger('click')
  expect(wrapper.get('.result-toggle').attributes('aria-expanded')).toBe('true')
  expect(wrapper.get('.result-toggle svg path').attributes('d')).toBe('m3 10 5-5 5 5')
  wrapper.unmount()
  height.mockRestore()
  scroll.mockRestore()
})

it('hides only a whitespace-equivalent heading while collapsed and preserves the citation', async () => {
  vi.stubGlobal('onUnmounted', () => {})
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(80)
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(160)
  const text = '  Điều  1.\tThuế  \r\nNội dung thuế áp dụng.'
  const hit = { doc: 'vn:vbpl:1', handle: 'vn:vbpl:1#dieu-1', citation: 'Điều 1', path: [{ heading: 'Thuế' }], text, complete: true }
  const documents = { 'vn:vbpl:1': { number: '1/2099', type: 'Luật', status: { current: true }, text_source: 'source_tree' as const, text_complete: true } }
  const wrapper = mount(SearchResults, { props: { result: { abstain: false, hits: [hit], documents }, query: 'thuế áp dụng' }, global: { stubs: { NuxtLink: true } } })
  await flushPromises()
  expect(wrapper.get('.result-text').text()).toBe('Nội dung thuế áp dụng.')
  expect(wrapper.get('mark').text()).toBe('thuế áp dụng')
  await wrapper.get('.result-actions button').trigger('click')
  expect(writeText).toHaveBeenCalledWith('Điều 1 Luật 1/2099')
  await wrapper.get('.result-toggle').trigger('click')
  expect(wrapper.get('.result-text').element.textContent).toBe(text)
  expect(wrapper.get('mark').text()).toBe('thuế áp dụng')
  await wrapper.get('.result-toggle').trigger('click')
  expect(wrapper.get('.result-text').text()).toBe('Nội dung thuế áp dụng.')
  wrapper.unmount()
  height.mockRestore()
  scroll.mockRestore()
})

it('shows the first line when its composition differs or it is another heading', async () => {
  const hit = { citation: 'Điều 1', path: [{ heading: 'Thuế' }], text: 'Điều 1. Thuế\nNội dung thuế áp dụng.', complete: true }
  const wrapper = mount(SearchResults, { props: { result: { abstain: false, hits: [hit] }, query: 'thuế áp dụng' }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('.result-text').text()).toBe(hit.text)
  expect(wrapper.get('mark').text()).toBe('thuế áp dụng')
  await wrapper.setProps({ result: { abstain: false, hits: [{ ...hit, text: 'Một tiêu đề khác\nNội dung thuế áp dụng.' }] } })
  expect(wrapper.get('.result-text').text()).toBe('Một tiêu đề khác\nNội dung thuế áp dụng.')
})

it('hides a repeated home heading while collapsed and preserves the full text and citation', async () => {
  vi.stubGlobal('onUnmounted', () => {})
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  const height = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(80)
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockReturnValue(160)
  const text = '  Điều  1.\tThuế  \r\nNội dung thuế áp dụng.'
  const hit = { doc: 'vn:vbpl:1', handle: 'vn:vbpl:1#dieu-1', citation: 'Điều 1', path: [{ heading: 'Thuế' }], text, complete: true }
  const documents = { 'vn:vbpl:1': { number: '1/2099', type: 'Luật', status: { current: true }, text_source: 'source_tree' as const, text_complete: true } }
  const wrapper = mount(ResultCard, { props: { home: true, hit, documents }, global: { stubs: { NuxtLink: true } } })
  await flushPromises()
  expect(wrapper.get('.result-text').text()).toBe('Nội dung thuế áp dụng.')
  await wrapper.get('.result-card-footer .result-actions button').trigger('click')
  expect(writeText).toHaveBeenCalledWith('Điều 1 Luật 1/2099')
  await wrapper.get('.result-toggle').trigger('click')
  expect(wrapper.get('.result-text').element.textContent).toBe(text)
  await wrapper.get('.result-toggle').trigger('click')
  await wrapper.setProps({ hit: { ...hit, text: 'Một tiêu đề khác\nNội dung thuế áp dụng.' } })
  expect(wrapper.get('.result-text').text()).toBe('Một tiêu đề khác\nNội dung thuế áp dụng.')
  wrapper.unmount()
  height.mockRestore()
  scroll.mockRestore()
})

it('shows the source host only for an https source', async () => {
  const doc = { number: '1/2099', status: { current: true }, text_source: 'source_tree' as const, text_complete: true, source_url: 'https://example.invalid/path' }
  const result = { abstain: false, hits: [{ doc: 'vn:vbpl:1', complete: true }], documents: { 'vn:vbpl:1': doc } }
  const wrapper = mount(SearchResults, { props: { result }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('.result-source').text()).toContain('Source: example.invalid')
  expect(wrapper.get('.result-source a').attributes('href')).toBe(doc.source_url)
  await wrapper.setProps({ result: { ...result, documents: { 'vn:vbpl:1': { ...doc, source_url: 'http://example.invalid/path' } } } })
  expect(wrapper.find('.result-source').exists()).toBe(false)
})
