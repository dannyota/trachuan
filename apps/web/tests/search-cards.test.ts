// @vitest-environment happy-dom
import { computed, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import SearchResults from '../app/components/SearchResults.vue'

beforeEach(() => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
})
afterEach(() => vi.unstubAllGlobals())

it('shows hit gaps beside the stamp and counts them in the summary', () => {
  const wrapper = mount(SearchResults, { props: { result: { release: 'r1', abstain: false, hits: [{ doc: 'vn:vbpl:1', citation: 'Điều 1', complete: true, gaps: [{ kind: 'validity_conflict', message: 'not used' }, { kind: 'missing_effective_date' }, { kind: 'unrecognized_code' }, { kind: 'missing_effective_date' }, { kind: 'consolidated_status_unordered' }, { kind: 'consolidated_status_unresolved' }, { kind: 'ambiguous_tree_text' }, { kind: '__proto__' }] }] } }, global: { stubs: ['NuxtLink'] } })
  const card = wrapper.find('.search-card')
  expect(card.text()).toContain('Document validity is unresolved')
  expect(card.text()).toContain('Effective start date is missing')
  expect(card.text()).toContain('unrecognized_code')
  expect(card.text()).toContain('__proto__')
  expect(card.text()).toContain('Consolidated document status could not be ordered')
  expect(card.text()).toContain('Consolidated document status could not be resolved')
  expect(card.text()).toContain('Source tree text overlaps ambiguously')
  expect(card.text()).not.toContain('not used')
  expect(wrapper.find('.search-summary').text()).toContain('Known gaps: 8')
  expect(wrapper.find('.search-summary').text()).toContain('Effective start date is missing.: 2')
})

it('identifies laws with identical citations and omits invalid links', () => {
  const doc = { status: { current: true }, text_source: 'source_tree' as const, text_complete: true }
  const wrapper = mount(SearchResults, { props: { result: { release: 'r1', abstain: false, documents: { 'vn:vbpl:one': { ...doc, number: '11/2024', title: 'First law' }, 'vn:vbpl:two': { ...doc, number: '22/2024', title: 'Second law' } }, hits: [{ doc: 'vn:vbpl:one', citation: 'Điều 1', complete: true }, { doc: 'vn:vbpl:two', citation: 'Điều 1', complete: true }, { doc: '../bad', citation: 'Điều 1', complete: true }] } }, global: { stubs: ['NuxtLink'] } })
  const cards = wrapper.findAll('.search-card')
  expect(cards[0]!.text()).toContain('11/2024')
  expect(cards[0]!.text()).toContain('First law')
  expect(cards[0]!.find('.document-number').text()).toBe('11/2024')
  expect(cards[1]!.text()).toContain('22/2024')
  expect(cards[1]!.text()).toContain('Second law')
  expect(cards[2]!.text()).toContain('../bad')
  expect(cards[2]!.find('nuxt-link-stub').exists()).toBe(false)
})
