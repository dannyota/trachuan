// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Search from '../app/pages/vn/search.vue'
import { createApi } from '../app/api/client'
import { createPinia, setActivePinia } from 'pinia'
import { useSearchDraftStore } from '../app/stores/search-draft'

const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'content-type': 'application/json' } })
const page = { release: '20261111_1', next_cursor: 'cursor-a', abstain: false, hits: [{ doc: 'vn:vbpl:1' }] }
const expired = () => json({ error: { code: 'release_expired', message: 'expired' } }, 409)
function setup(fetch: typeof globalThis.fetch, route: { query: Record<string, string> } = { query: {} }) {
  const session = { account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'token-a', status: 'signed-in' as const }
  const api = createApi({ origin: 'http://localhost', session, fetch: input => (input as Request).url.endsWith('/api/v1/status') ? Promise.resolve(json({ doc_types: [{ value: 'Type fixture', count: 2 }], issuers: [{ value: 'Issuer fixture', count: 1 }] })) : fetch(input) })
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('useRoute', () => route)
  vi.stubGlobal('useRouter', () => ({ replace: vi.fn() }))
  vi.stubGlobal('nextTick', nextTick)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useApi', () => api)
  setActivePinia(createPinia())
  const wrapper = mount(Search, { global: { stubs: { NuxtLayout: { template: '<div><slot name="header" /><slot /></div>' }, SearchResults: { props: ['result'], emits: ['clearFilters'], template: `<div class="result-stub search-results"><h1 tabindex="-1">{{ result.hits?.length }}</h1><button @click="$emit('clearFilters')">Clear filters</button></div>` }, SearchNodePicker: true } } })
  return { wrapper, session }
}
async function submit(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('textarea').setValue('fixture query')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}
async function more(wrapper: ReturnType<typeof mount>) {
  await wrapper.findAll('button').find(button => button.text() === 'Show more results')!.trigger('click')
  await flushPromises()
}
afterEach(() => vi.unstubAllGlobals())
it('does not resend the query after the account changes during a pinned search', async () => {
  const calls: Request[] = []
  let finish!: (response: Response) => void
  const { wrapper, session } = setup(async input => {
    calls.push(input as Request)
    if (calls.length === 1) return json(page)
    if (calls.length === 2) return new Promise(resolve => { finish = resolve })
    return json(page)
  })
  await submit(wrapper)
  await more(wrapper)
  session.account = { ...session.account, id: 2 }
  session.csrfToken = 'token-b'
  finish(expired())
  await flushPromises()
  expect(calls).toHaveLength(2)
  expect(calls.map(call => call.headers.get('x-csrf-token'))).toEqual(['token-a', 'token-a'])
  wrapper.unmount()
})
it('aborts a pending search on unmount and drops a late expiration response', async () => {
  const calls: Request[] = []
  let finish!: (response: Response) => void
  const { wrapper } = setup(async input => {
    calls.push(input as Request)
    if (calls.length === 1) return json(page)
    if (calls.length === 2) return new Promise(resolve => { finish = resolve })
    return json(page)
  })
  await submit(wrapper)
  await more(wrapper)
  wrapper.unmount()
  expect(calls[1]!.signal.aborted).toBe(true)
  finish(expired())
  await flushPromises()
  expect(calls).toHaveLength(2)
})
it('stops an expired cursor without restarting the search', async () => {
  const calls: { body: Record<string, unknown>, csrf: string | null }[] = []
  const { wrapper, session } = setup(async input => {
    const request = input as Request
    calls.push({ body: await request.json(), csrf: request.headers.get('x-csrf-token') })
    return calls.length === 2 ? expired() : json(page)
  })
  await submit(wrapper)
  session.csrfToken = 'new-token'
  await more(wrapper)
  expect(calls).toHaveLength(2)
  expect(calls.map(call => call.csrf)).toEqual(['token-a', 'token-a'])
  expect(calls[1]!.body).toMatchObject({ release: '20261111_1', cursor: 'cursor-a', limit: 20 })
  expect(wrapper.findAll('button').some(button => button.text() === 'Show more results')).toBe(false)
  wrapper.unmount()
})

it('appends cursor pages and stops at the end', async () => {
  const calls: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => {
    calls.push(await (input as Request).json())
    return json(calls.length === 1 ? page : { release: '20261111_1', abstain: false, hits: [{ doc: 'vn:vbpl:2' }] })
  })
  await submit(wrapper)
  await more(wrapper)
  expect(calls[1]).toMatchObject({ cursor: 'cursor-a', release: '20261111_1', limit: 20 })
  expect(wrapper.get('.result-stub h1').text()).toBe('2')
  expect(wrapper.findAll('button').some(button => button.text() === 'Show more results')).toBe(false)
  wrapper.unmount()
})

it('does not refresh CSRF or retry after unmount during a pending request', async () => {
  const calls: Request[] = []
  let finish!: (response: Response) => void
  const { wrapper } = setup(async input => {
    calls.push(input as Request)
    return new Promise(resolve => { finish = resolve })
  })
  await submit(wrapper)
  wrapper.unmount()
  finish(json({ error: { code: 'csrf_rejected', message: 'rejected' } }, 403))
  await flushPromises()
  expect(calls).toHaveLength(1)
})

it('loads facets and sends selected filters without writing them to browser state', async () => {
  const calls: { url: string, body?: Record<string, unknown> }[] = []
  const { wrapper } = setup(async input => {
    const request = input as Request
    calls.push({ url: request.url, ...(request.method === 'POST' ? { body: await request.json() } : {}) })
    return request.url.endsWith('/api/v1/status') ? json({ doc_types: [{ value: 'Type fixture', count: 2 }], issuers: [{ value: 'Issuer fixture', count: 1 }] }) : json(page)
  })
  await flushPromises()
  await wrapper.findAll('.facet-option').find(label => label.text().includes('Type fixture'))!.get('input').setValue(true)
  await wrapper.findAll('.facet-option').find(label => label.text().includes('Issuer fixture'))!.get('input').setValue(true)
  await submit(wrapper)
  expect(calls.find(call => call.body)?.body).toMatchObject({ doc_types: ['Type fixture'], issuers: ['Issuer fixture'] })
  expect([location.href, JSON.stringify(history.state), JSON.stringify(localStorage), JSON.stringify(sessionStorage), document.title].join(' ')).not.toMatch(/Type fixture|Issuer fixture/)
  wrapper.unmount()
})

it('reads a valid topic node once without persisting form state', async () => {
  const route = { query: { node: '1.2' } }
  const calls: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => { calls.push(await (input as Request).json()); return json(page) }, route)
  route.query.node = '3.4'
  await submit(wrapper)
  expect(calls[0]?.nodes).toEqual(['1.2'])
  expect([location.href, JSON.stringify(history.state), JSON.stringify(localStorage), JSON.stringify(sessionStorage), document.title].join(' ')).not.toContain('3.4')
  wrapper.unmount()
})

it('keeps paging available after a temporary error', async () => {
  let count = 0
  const { wrapper } = setup(async () => ++count === 1 ? json(page) : json({ error: { code: 'invalid_request', message: 'error' } }, 400))
  await submit(wrapper)
  await more(wrapper)
  expect(wrapper.findAll('button').some(button => button.text() === 'Show more results')).toBe(true)
  wrapper.unmount()
})

it('limits each facet selection to ten values', async () => {
  const facets = Array.from({ length: 11 }, (_, i) => ({ value: `facet-${i}`, count: 1 }))
  const session = { account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'token', status: 'signed-in' as const }
  const bodies: Record<string, unknown>[] = []
  const api = createApi({ origin: 'http://localhost', session, fetch: async input => {
    const request = input as Request
    if (request.method === 'GET') return json({ doc_types: facets, issuers: facets })
    bodies.push(await request.json())
    return json(page)
  } })
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('useRoute', () => ({ query: {} }))
  vi.stubGlobal('useRouter', () => ({ replace: vi.fn() }))
  vi.stubGlobal('useApi', () => api)
  setActivePinia(createPinia())
  const wrapper = mount(Search, { global: { stubs: { NuxtLayout: { template: '<div><slot name="header" /><slot /></div>' }, SearchResults: true, SearchNodePicker: true } } })
  await flushPromises()
  for (const button of wrapper.findAll('.filter-more')) await button.trigger('click')
  const fieldsets = wrapper.findAll('.search-filters > fieldset')
  for (const fieldset of fieldsets.slice(1, 3)) {
    const checkboxes = fieldset.findAll('input[type="checkbox"]')
    for (const checkbox of checkboxes.slice(0, 10)) await checkbox.setValue(true)
    expect(checkboxes[10]!.attributes('disabled')).toBeDefined()
  }
  await submit(wrapper)
  expect(bodies[0]?.doc_types).toHaveLength(10)
  expect(bodies[0]?.issuers).toHaveLength(10)
  wrapper.unmount()
})

it('consumes a home draft once and submits its filter without persisting the query', async () => {
  const calls: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => { calls.push(await (input as Request).json()); return json(page) })
  wrapper.unmount()
  const pinia = createPinia()
  setActivePinia(pinia)
  useSearchDraftStore().draft = { query: 'draft fixture', inForceOnly: false }
  const mounted = mount(Search, { global: { plugins: [pinia], stubs: { NuxtLayout: { template: '<div><slot name="header" /><slot /></div>' }, SearchResults: true, SearchNodePicker: true } } })
  await flushPromises()
  expect(calls).toHaveLength(1)
  expect(calls[0]).toMatchObject({ query: 'draft fixture', in_force_only: false })
  expect(useSearchDraftStore().draft).toBeNull()
  expect([location.href, JSON.stringify(history.state), JSON.stringify(localStorage), JSON.stringify(sessionStorage)].join(' ')).not.toContain('draft fixture')
  mounted.unmount()
})

it('keeps filter changes local until the next submission', async () => {
  const calls: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => { calls.push(await (input as Request).json()); return json(page) })
  await flushPromises()
  await wrapper.findAll('.facet-option').find(label => label.text().includes('Type fixture'))!.get('input').setValue(true)
  expect(calls).toHaveLength(0)
  await submit(wrapper)
  expect(calls[0]?.doc_types).toEqual(['Type fixture'])
  wrapper.unmount()
})

it('keeps the previous result area while a new search loads', async () => {
  let count = 0
  let finish!: (response: Response) => void
  const { wrapper } = setup(async () => ++count === 1 ? json(page) : new Promise(resolve => { finish = resolve }))
  await submit(wrapper)
  await wrapper.get('form').trigger('submit')
  expect(wrapper.find('.search-results-pending').exists()).toBe(true)
  expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  finish(json({ release: '20990101', abstain: false, hits: [] }))
  await flushPromises()
  expect(wrapper.find('.search-results-pending').exists()).toBe(false)
  wrapper.unmount()
})

it('submits on Enter but not Shift+Enter or during text composition', async () => {
  const calls: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => { calls.push(await (input as Request).json()); return json(page) })
  const field = wrapper.get('textarea')
  await field.setValue('fixture question')
  await field.trigger('keydown', { key: 'Enter', shiftKey: true })
  await field.trigger('keydown', { key: 'Enter', isComposing: true })
  await flushPromises()
  expect(calls).toHaveLength(0)
  await field.trigger('keydown', { key: 'Enter' })
  await flushPromises()
  expect(calls).toHaveLength(1)
  expect(calls[0]?.query).toBe('fixture question')
  wrapper.unmount()
})


it('retries the failed cursor request and appends its result', async () => {
  const bodies: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => {
    bodies.push(await (input as Request).json())
    if (bodies.length === 2) return json({}, 503)
    return json(bodies.length === 1 ? page : { release: page.release, abstain: false, hits: [{ doc: 'vn:vbpl:2' }] })
  })
  document.body.appendChild(wrapper.element)
  await submit(wrapper)
  await more(wrapper)
  await wrapper.get('[role="alert"] button').trigger('click')
  await flushPromises()
  expect(document.activeElement).toBe(wrapper.get('.result-stub h1').element)
  expect(bodies[2]).toEqual(bodies[1])
  expect(wrapper.get('.result-stub').text()).toContain('2')
  wrapper.unmount()
})

it('clears filters using the submitted query when the field is empty', async () => {
  const bodies: Record<string, unknown>[] = []
  const { wrapper } = setup(async input => { bodies.push(await (input as Request).json()); return json({ release: page.release, abstain: true, hits: [] }) })
  document.body.appendChild(wrapper.element)
  await submit(wrapper)
  await wrapper.get('textarea').setValue('')
  await wrapper.get('.result-stub button').trigger('click')
  await flushPromises()
  expect(document.activeElement).toBe(wrapper.get('.result-stub h1').element)
  expect(bodies).toHaveLength(2)
  expect(bodies[1]).toMatchObject({ query: 'fixture query', in_force_only: false })
  expect(bodies[1]).not.toHaveProperty('nodes')
  wrapper.unmount()
})

it('focuses the error heading when a retry fails again', async () => {
  const { wrapper } = setup(async () => json({}, 503))
  document.body.appendChild(wrapper.element)
  await submit(wrapper)
  await wrapper.get('[role="alert"] button').trigger('click')
  await flushPromises()
  expect(document.activeElement).toBe(wrapper.get('[role="alert"] h1').element)
  wrapper.unmount()
})

it('replaces disallowed search query parameters through the router', async () => {
  const replace = vi.fn()
  const { wrapper } = setup(async () => json(page))
  wrapper.unmount()
  vi.stubGlobal('useRoute', () => ({ query: { node: '1', q: 'private' }, hash: '' }))
  vi.stubGlobal('useRouter', () => ({ replace }))
  const mounted = mount(Search, { global: { stubs: { NuxtLayout: { template: '<div><slot name="header" /><slot /></div>' }, SearchResults: true, SearchNodePicker: true } } })
  await flushPromises()
  expect(replace).toHaveBeenCalledWith(expect.objectContaining({ query: { node: '1' } }))
  mounted.unmount()
})
