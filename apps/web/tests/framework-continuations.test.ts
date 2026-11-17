// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, ref, Suspense } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import FrameworkRead from '../app/components/FrameworkRead.vue'
import FrameworkGaps from '../app/components/FrameworkGaps.vue'
import FrameworkUnits from '../app/components/FrameworkUnits.vue'

const response = (data: unknown) => ({ data, response: { status: 200 } })
const failure = (code: string) => ({ error: { error: { code } }, response: { status: 409 } })
const head = { kind: 'head', head: { handle: 'fw:fw-alpha:2.1#A', citation: 'A', kind: 'control', access: 'licensed', text_withheld: true } }
function setup(component: typeof FrameworkRead | typeof FrameworkGaps, replies: unknown[]) {
  const calls: unknown[] = []
  const notFound = vi.fn()
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('useFrameworkPage', () => ({ language: ref('en'), notFound, api: { client: { GET: async (_path: string, options: unknown) => { calls.push(options); return replies.shift() } } } }))
  const root = defineComponent({ render: () => h(Suspense, null, { default: () => h(component, { handle: 'fw:fw-alpha:2.1#A', kind: 'control' }) }) })
  return { wrapper: mount(root, { global: { components: { FrameworkUnits }, stubs: { NuxtLink: true } } }), calls, notFound }
}
afterEach(() => vi.unstubAllGlobals())

it.each(['invalid_next', 'release_expired'])('restarts a read when its continuation returns %s', async (code) => {
  const { wrapper, calls } = setup(FrameworkRead, [
    response({ release: '20990101', items: [{ units: [head], next: 'next-a' }] }),
    response({ release: '20990101', items: [{ error: code }] }),
    response({ release: '20990102', items: [{ units: [head], complete: true }] }),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('The data was updated; the page starts again from the top.')
  expect(calls).toHaveLength(3)
  expect(calls[1]).toMatchObject({ params: { query: { next: 'next-a', release: '20990101' } } })
  expect(wrapper.find('button').exists()).toBe(false)
})

it('restarts expired gaps with the same version selector', async () => {
  const { wrapper, calls } = setup(FrameworkGaps, [
    response({ release: '20990101', gaps: [{ kind: 'title_missing', detail: 'first' }], next_cursor: 'cursor-a' }),
    failure('cursor_expired'),
    response({ release: '20990102', gaps: [{ kind: 'unknown_kind', detail: 'second' }] }),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('The results changed. Please search again.')
  expect(wrapper.text()).toContain('unknown_kind')
  expect(wrapper.text()).not.toContain('first')
  expect(calls[2]).toMatchObject({ params: { query: { versions: ['fw:fw-alpha:2.1#A'] } } })
})

it.each(['unit_too_large', 'invalid_item'] as const)('shows the fixed %s message and stops', async (code) => {
  const { wrapper } = setup(FrameworkRead, [response({ release: '20990101', items: [{ error: code, complete: false, units: [head] }] })])
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toBe(code === 'invalid_item' ? 'This item could not be read.' : 'Part of this item is too large to show.')
  expect(wrapper.find('button').exists()).toBe(false)
})

it('appends read units and shows the access notice', async () => {
  const { wrapper, calls } = setup(FrameworkRead, [
    response({ release: '20990101', access_unavailable: true, items: [{ units: [head], next: 'next-a' }] }),
    response({ release: '20990101', access_unavailable: true, items: [{ units: [{ kind: 'body', text: 'Synthetic body' }], complete: true }] }),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Access could not be checked right now; this shows the free part.')
  expect(wrapper.text()).toContain('Synthetic body')
  expect(calls).toHaveLength(2)
})

it('appends gap pages with the same selector', async () => {
  const { wrapper, calls } = setup(FrameworkGaps, [
    response({ release: '20990101', gaps: [{ kind: 'title_missing', detail: 'first' }], next_cursor: 'cursor-a' }),
    response({ release: '20990101', gaps: [{ kind: 'mapping_unresolved', detail: 'second' }] }),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('first')
  expect(wrapper.text()).toContain('second')
  expect(calls[1]).toMatchObject({ params: { query: { versions: ['fw:fw-alpha:2.1#A'], cursor: 'cursor-a', release: '20990101' } } })
})

it.each(['invalid_handle', 'not_found'])('uses the fixed 404 for read item %s', async (code) => {
  const { notFound } = setup(FrameworkRead, [response({ release: '20990101', items: [{ error: code, complete: false }] })])
  await flushPromises()
  expect(notFound).toHaveBeenCalledOnce()
})

it.each(['unavailable', 'rate_limited'])('shows %s for a gap error without a continuation button', async (code) => {
  const { wrapper } = setup(FrameworkGaps, [failure(code)])
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toBe(code === 'unavailable' ? 'Please try again later.' : 'Too many requests. Please try again later.')
  expect(wrapper.find('button').exists()).toBe(false)
})

it.each(['invalid_handle', 'not_found'])('uses the fixed 404 for a read response error %s', async (code) => {
  const { notFound } = setup(FrameworkRead, [failure(code)])
  await flushPromises()
  expect(notFound).toHaveBeenCalledOnce()
})
