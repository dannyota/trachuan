// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import CoverageFacts from '../app/components/CoverageFacts.vue'

afterEach(() => vi.unstubAllGlobals())

it('formats coverage and groups gaps without exposing their codes', () => {
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  const status = { release: '20260101_1', documents: 1200, provisions: 2500, nodes: 10, newest_source_date: '2026-01-01', gaps: { missing_attachment: 3 } }
  const wrapper = mount(CoverageFacts, { props: { status, showGaps: true } })
  expect(wrapper.text()).toContain('1.200')
  expect(wrapper.text()).toContain('01/01/2026')
  expect(wrapper.text()).toContain('3 trường hợp')
  expect(wrapper.text()).not.toContain('missing_attachment')
  expect(wrapper.text()).not.toContain('20260101_1')
})
