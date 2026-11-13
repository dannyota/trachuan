// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import ReaderRail from '../app/components/ReaderRail.vue'

afterEach(() => vi.unstubAllGlobals())
it('loads relations on expansion and continues with the returned token', async () => {
  const calls: unknown[] = []
  const post = async (_path: string, options: unknown) => {
    calls.push(options)
    return { data: { release: '20990101_1', items: [{ complete: calls.length > 1, next: calls.length === 1 ? 'next-token' : undefined, units: [{ kind: 'relation', relation: { dir: 'out', kind: 'legal_basis', number: calls.length === 1 ? 'A' : 'B', in_release: false, evidence_complete: true } }] }] } }
  }
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useApi', () => ({ captureAction: () => ({ accountId: 1, csrfToken: 'csrf', bound: false }), forAction: () => ({ POST: post }), session: { account: { id: 1 } } }))
  const wrapper = mount(ReaderRail, { props: { document: { status: { current: true }, text_source: 'none', text_complete: false, related: [], related_counts: {} }, release: '20990101_1', documentKey: 'vn:vbpl:1', gaps: [] } })
  expect(calls).toHaveLength(0)
  const details = wrapper.get('details')
  details.element.open = true
  await details.trigger('toggle')
  await flushPromises()
  expect(calls[0]).toMatchObject({ body: { country: 'vn', release: '20990101_1', items: [{ handle: 'vn:vbpl:1', relations: true }] }, params: { header: { 'X-CSRF-Token': 'csrf' } } })
  expect(wrapper.text()).toContain('A')
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(calls[1]).toMatchObject({ body: { country: 'vn', release: '20990101_1', items: [{ next: 'next-token' }] } })
  expect(wrapper.text()).toContain('B')
})
