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
  expect(wrapper.text()).toContain('The list was reloaded from the top.')
  expect(wrapper.text()).not.toContain('The results changed. Please search again.')
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

it.each(['release_expired', 'invalid_next'])('restarts after call-level %s on a continuation', async (code) => {
  const { wrapper, calls } = setup(FrameworkRead, [
    response({ release: '20990101', items: [{ units: [head], next: 'next-a' }] }),
    failure(code),
    response({ release: '20990102', items: [{ units: [head], complete: true }] }),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(calls).toHaveLength(3)
  expect(calls[2]).toMatchObject({ params: { query: {} } })
  expect(wrapper.text()).toContain('The data was updated; the page starts again from the top.')
  expect(wrapper.find('button').exists()).toBe(false)
})

it('does not restart again if the fresh read fails', async () => {
  const { wrapper, calls } = setup(FrameworkRead, [
    response({ release: '20990101', items: [{ units: [head], next: 'next-a' }] }),
    failure('release_expired'),
    failure('release_expired'),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(calls).toHaveLength(3)
  expect(wrapper.get('[role="alert"]').text()).toBe('The release has expired.')
  expect(wrapper.find('button').exists()).toBe(false)
})

it('shows the item error if the fresh read also fails', async () => {
  const { wrapper, calls } = setup(FrameworkRead, [
    response({ release: '20990101', items: [{ units: [head], next: 'next-a' }] }),
    response({ release: '20990101', items: [{ error: 'release_expired' }] }),
    response({ release: '20990101', items: [{ error: 'release_expired' }] }),
  ])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(calls).toHaveLength(3)
  expect(wrapper.get('[role="alert"]').text()).toBe('The release has expired.')
})

const gapPage = (kind: string, next?: string) => response({ release: '20990101', gaps: [{ kind, detail: kind }], next_cursor: next })
const reloadedText = 'The list was reloaded from the top.'

it('shows no restart notice when the gaps restart fails', async () => {
  const { wrapper } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('cursor_expired'), failure('unavailable')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  expect(wrapper.text()).not.toContain(reloadedText)
})

it('clears the gaps restart notice after a later page loads', async () => {
  const { wrapper } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('cursor_expired'), gapPage('unknown_kind', 'cursor-b'), gapPage('ocr_blocked')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain(reloadedText)
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('ocr_blocked')
  expect(wrapper.text()).not.toContain(reloadedText)
})

it('drops the gaps restart notice when a second restart fails', async () => {
  const { wrapper } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('cursor_expired'), gapPage('unknown_kind', 'cursor-b'), failure('cursor_expired'), failure('unavailable')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain(reloadedText)
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  expect(wrapper.text()).not.toContain(reloadedText)
})

it('restarts the gaps list from the top when the release expired on show more', async () => {
  const { wrapper, calls } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('release_expired'), gapPage('unknown_kind')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain(reloadedText)
  expect(wrapper.text()).toContain('unknown_kind')
  expect(wrapper.text()).not.toContain('title_missing')
  expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  const query = (calls[2] as { params: { query: Record<string, unknown> } }).params.query
  expect(query).not.toHaveProperty('cursor')
  expect(query).not.toHaveProperty('release')
})

it('shows no restart notice when the gaps restart after release expiry fails', async () => {
  const { wrapper, calls } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('release_expired'), failure('unavailable')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  expect(wrapper.text()).not.toContain(reloadedText)
  expect(calls).toHaveLength(3)
})

it('shows the error for a first-page gaps release expiry without retrying', async () => {
  const { wrapper, calls } = setup(FrameworkGaps, [failure('release_expired')])
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toBe('The release has expired.')
  expect(calls).toHaveLength(1)
})

it('keeps the gaps rows and button after a failed restart on release expiry, then recovers', async () => {
  const { wrapper } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('release_expired'), failure('unavailable'), failure('release_expired'), gapPage('unknown_kind')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  expect(wrapper.text()).not.toContain(reloadedText)
  expect(wrapper.text()).toContain('title_missing')
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain(reloadedText)
  expect(wrapper.text()).toContain('unknown_kind')
  expect(wrapper.text()).not.toContain('title_missing')
  expect(wrapper.find('[role="alert"]').exists()).toBe(false)
})

it('clears the gaps rows before a failed restart on cursor expiry', async () => {
  const { wrapper } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('cursor_expired'), failure('unavailable')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  expect(wrapper.text()).not.toContain('title_missing')
})

it('shows the fixed not-found page when the gaps restart returns invalid_request', async () => {
  const { wrapper, notFound } = setup(FrameworkGaps, [gapPage('title_missing', 'cursor-a'), failure('release_expired'), failure('invalid_request')])
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(notFound).toHaveBeenCalledTimes(1)
  expect(wrapper.text()).not.toContain(reloadedText)
})
