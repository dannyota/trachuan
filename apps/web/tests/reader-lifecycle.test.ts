// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

it('continues through outline pages while the sentinel stays visible', async () => {
  const calls = setupReader([
    ...[1, 2, 3].map(i => ({ data: { release: '20261111_1', item: { complete: false, next: `p${i}`, units: [{ kind: 'outline', anchor: `a${i}`, label: `Outline ${i}` }] } } })),
    { data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', text: 'Final provision' }] } } },
  ])
  class Observer {
    callback: IntersectionObserverCallback
    fired = false
    constructor(callback: IntersectionObserverCallback) { this.callback = callback }
    observe() { if (!this.fired) { this.fired = true; queueMicrotask(() => this.callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver)) } }
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  const wrapper = mount(Reader)
  for (let i = 0; i < 8; i++) await flushPromises()
  expect(wrapper.text()).toContain('Final provision')
  expect(calls).toHaveLength(4)
  wrapper.unmount()
})

it('aborts a pending request and does not continue or scroll after unmount', async () => {
  setupReader([])
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '#target' }))
  let resolve!: (value: unknown) => void
  let signal: AbortSignal | undefined
  const get = vi.fn((_path: string, options: { signal?: AbortSignal }) => {
    signal = options.signal
    return new Promise(r => { resolve = r })
  })
  vi.stubGlobal('useApi', () => ({ client: { GET: get } }))
  const scroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const wrapper = mount(Reader)
  await flushPromises()
  wrapper.unmount()
  resolve({ data: { release: '20261111_1', item: { complete: false, next: 'next', units: [] } } })
  await flushPromises()
  expect(signal?.aborted).toBe(true)
  expect(get).toHaveBeenCalledTimes(1)
  expect(scroll).not.toHaveBeenCalled()
})

it('aborts a pending request when an invalid release query changes', async () => {
  setupReader([])
  const { reactive } = await import('vue')
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: { release: 'invalid-one' }, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  const signals: AbortSignal[] = []
  const get = vi.fn((_path: string, options: { signal: AbortSignal }) => {
    signals.push(options.signal)
    return new Promise(() => {})
  })
  vi.stubGlobal('useApi', () => ({ client: { GET: get } }))
  const wrapper = mount(Reader)
  await flushPromises()
  route.query.release = 'invalid-two'
  await flushPromises()
  expect(signals[0]?.aborted).toBe(true)
  expect(signals).toHaveLength(2)
  wrapper.unmount()
})

it('passes the request signal through the API client to fetch', async () => {
  const { createApi } = await import('../app/api/client')
  const controller = new AbortController()
  let received: AbortSignal | undefined
  const transport: typeof fetch = async (request) => {
    received = (request as Request).signal
    return new Response(JSON.stringify({ release: '20261111_1', item: { complete: true } }), { headers: { 'content-type': 'application/json' } })
  }
  const api = createApi({ origin: 'http://localhost', fetch: transport })
  await api.client.GET('/api/v1/documents/{key}', { params: { path: { key: 'vn:vbpl:1' } }, signal: controller.signal })
  controller.abort()
  expect(received?.aborted).toBe(true)
})

it('does not redirect to sign-in for a response after cancellation', async () => {
  const { createApi } = await import('../app/api/client')
  const controller = new AbortController()
  let resolve!: (response: Response) => void
  const transport: typeof fetch = () => new Promise(r => { resolve = r })
  const onUnauthorized = vi.fn()
  const api = createApi({ origin: 'http://localhost', fetch: transport, onUnauthorized })
  const request = api.client.GET('/api/v1/documents/{key}', { params: { path: { key: 'vn:vbpl:1' } }, signal: controller.signal })
  controller.abort()
  resolve(new Response(null, { status: 401 }))
  await request
  expect(onUnauthorized).not.toHaveBeenCalled()
})

it('does not scroll when the router has left the reader before unmount', async () => {
  setupReader([])
  const { reactive, ref } = await import('vue')
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '#missing' })
  const currentRoute = ref({ path: '/vn/documents/vn:vbpl:1', query: {}, hash: '#missing' })
  vi.stubGlobal('useRoute', () => route)
  vi.stubGlobal('useRouter', () => ({ currentRoute }))
  vi.stubGlobal('useApi', () => ({ client: { GET: () => new Promise(() => {}) } }))
  const scroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const wrapper = mount(Reader)
  await flushPromises()
  currentRoute.value = { path: '/connect', query: {}, hash: '' }
  route.hash = ''
  await flushPromises()
  expect(scroll).not.toHaveBeenCalled()
  wrapper.unmount()
})

it('scrolls to the top for an unknown anchor visited after initial load', async () => {
  setupReader([{ data: { release: '20261111_1', item: { complete: true, units: [] } } }])
  const { reactive } = await import('vue')
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  const scroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const wrapper = mount(Reader)
  await flushPromises()
  scroll.mockClear()
  route.hash = '#missing'
  await flushPromises()
  expect(scroll).toHaveBeenCalledWith(0, 0)
  wrapper.unmount()
})
it('uses a viewport-height observation band and updates it on resize', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [] } } }])
  const margins: string[] = []
  class Observer {
    constructor(_callback: IntersectionObserverCallback, options: IntersectionObserverInit) { margins.push(options.rootMargin || '') }
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  vi.stubGlobal('innerHeight', 720)
  const wrapper = mount(Reader)
  await flushPromises()
  expect(margins[1]).toBe('-72px 0px -540px 0px')
  vi.stubGlobal('innerHeight', 1000)
  window.dispatchEvent(new Event('resize'))
  expect(margins[2]).toBe('-100px 0px -750px 0px')
  wrapper.unmount()
})
