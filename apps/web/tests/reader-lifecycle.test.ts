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
it('rebuilds the heading observer when the viewport changes', async () => {
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
  expect(margins[1]).toBe('-1px 0px 0px 0px')
  const count = margins.length
  vi.stubGlobal('innerHeight', 1000)
  window.dispatchEvent(new Event('resize'))
  expect(margins).toHaveLength(count + 1)
  expect(margins.at(-1)).toBe('-1px 0px 0px 0px')
  wrapper.unmount()
})

it('focuses the new target after selecting a contents entry and changing the hash', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    { kind: 'outline', anchor: 'dieu-2', label: 'Điều 2' },
    { kind: 'provision', anchor: 'dieu-2', label: 'Điều 2', text: 'Synthetic text' },
  ] } } }])
  const { reactive } = await import('vue')
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  vi.stubGlobal('useRouter', () => ({ currentRoute: { get value() { return { path: '/vn/documents/vn:vbpl:1', query: {}, hash: route.hash } } }, push: async ({ hash }: { hash: string }) => { route.hash = hash } }))
  const wrapper = mount(Reader, { attachTo: document.body })
  await flushPromises()
  const link = wrapper.get('dialog a')
  await link.trigger('click')
  await flushPromises()
  expect(route.hash).toBe('#dieu-2')
  expect(document.activeElement).toBe(wrapper.get('[data-anchor="dieu-2"]').element)
  wrapper.unmount()
})
it('uses the measured header and compact bar heights for anchor offsets', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [] } } }])
  const shell = document.createElement('div')
  shell.className = 'site-layout'
  const header = document.createElement('header')
  header.className = 'site-header'
  shell.append(header)
  document.body.append(shell)
  let compactHeight = 0
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    return { height: this === header ? 73 : this.classList.contains('reader-compact') ? compactHeight : 0 } as DOMRect
  })
  let resize!: ResizeObserverCallback
  const margins: string[] = []
  class Intersection {
    constructor(_callback: IntersectionObserverCallback, options: IntersectionObserverInit) { margins.push(options.rootMargin || '') }
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal('innerHeight', 720)
  vi.stubGlobal('IntersectionObserver', Intersection)
  class Observer {
    constructor(callback: ResizeObserverCallback) { resize = callback }
    observe() {}
    disconnect() {}
  }
  vi.stubGlobal('ResizeObserver', Observer)
  const wrapper = mount(Reader, { attachTo: shell })
  await flushPromises()
  expect(shell.style.getPropertyValue('--reader-stack-height')).toBe('73px')
  expect(margins).toContain('-74px 0px 0px 0px')
  compactHeight = 60
  resize([], {} as ResizeObserver)
  expect(shell.style.getPropertyValue('--reader-stack-height')).toBe('133px')
  expect(margins.at(-1)).toBe('-134px 0px 0px 0px')
  wrapper.unmount()
  shell.remove()
})
it('tracks a heading after the hash moves it into the target band', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    { kind: 'outline', anchor: 'dieu-1', label: 'Điều 1' },
    { kind: 'outline', anchor: 'dieu-2', label: 'Điều 2' },
    { kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', text: 'First' },
    { kind: 'provision', anchor: 'dieu-2', label: 'Điều 2', text: 'Second' },
  ] } } }])
  const { reactive } = await import('vue')
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  const observers: Observer[] = []
  class Observer {
    observed = new Set<Element>()
    constructor(private callback: IntersectionObserverCallback) { observers.push(this) }
    observe(element: Element) { this.observed.add(element) }
    unobserve(element: Element) { this.observed.delete(element) }
    disconnect() { this.observed.clear() }
    intersect(element: Element) {
      if (this.observed.has(element)) this.callback([{ target: element, isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
    }
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  const wrapper = mount(Reader)
  await flushPromises()
  route.hash = '#dieu-2'
  await flushPromises()
  observers[1]!.intersect(wrapper.get('[data-spy-anchor="dieu-2"]').element)
  await flushPromises()
  expect(wrapper.get('.reader-toc-desktop [aria-current="location"]').text()).toBe('Điều 2')
  wrapper.unmount()
})
it('coalesces observer callbacks and tracks large position jumps in both directions', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    ...[1, 2, 3].map(i => ({ kind: 'outline', anchor: `dieu-${i}`, label: `Điều ${i}` })),
    ...[1, 2, 3].map(i => ({ kind: 'provision', anchor: `dieu-${i}`, label: `Điều ${i}`, text: 'Synthetic text' })),
  ] } } }])
  const shell = document.createElement('div')
  shell.className = 'site-layout'
  shell.innerHTML = '<header class="site-header"></header>'
  document.body.append(shell)
  let positions = [40, 500, 1000]
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const index = Number(this.dataset.spyAnchor?.slice(5)) - 1
    const top = Number.isNaN(index) ? 0 : positions[index]!
    return { top, bottom: top + 20, height: this.classList.contains('site-header') ? 73 : 0 } as DOMRect
  })
  const frames: FrameRequestCallback[] = []
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.push(callback); return frames.length })
  const cancel = vi.fn()
  vi.stubGlobal('cancelAnimationFrame', cancel)
  const observers: Observer[] = []
  class Observer {
    targets = new Set<Element>()
    constructor(private callback: IntersectionObserverCallback) { observers.push(this) }
    observe(element: Element) { this.targets.add(element) }
    unobserve(element: Element) { this.targets.delete(element) }
    disconnect() { this.targets.clear() }
    notify() { this.callback([], this as unknown as IntersectionObserver) }
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  const wrapper = mount(Reader, { attachTo: shell })
  await flushPromises()
  const selected = () => wrapper.find('.reader-toc-desktop [aria-current="location"]').text()
  expect(selected()).toBe('Điều 1')
  const second = wrapper.get('[data-spy-anchor="dieu-2"]').element
  const headingObserver = observers.findLast(observer => observer.targets.has(second))!
  positions = [-500, 70, 500]
  headingObserver.notify()
  headingObserver.notify()
  expect(frames).toHaveLength(1)
  expect(selected()).toBe('Điều 1')
  frames.shift()!(0)
  await flushPromises()
  expect(selected()).toBe('Điều 2')
  positions = [40, 75, 500]
  headingObserver.notify()
  frames.shift()!(0)
  await flushPromises()
  expect(selected()).toBe('Điều 1')
  positions = [100, 500, 1000]
  headingObserver.notify()
  frames.shift()!(0)
  await flushPromises()
  expect(wrapper.find('.reader-toc-desktop [aria-current="location"]').exists()).toBe(false)
  headingObserver.notify()
  wrapper.unmount()
  expect(cancel).toHaveBeenCalledOnce()
  shell.remove()
})
it('selects an anchored one-line heading in a tall viewport', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    ...[1, 2].map(i => ({ kind: 'outline', anchor: `dieu-${i}`, label: `Điều ${i}` })),
    ...[1, 2].map(i => ({ kind: 'provision', anchor: `dieu-${i}`, label: `Điều ${i}`, text: 'Synthetic text' })),
  ] } } }])
  vi.stubGlobal('innerHeight', 1440)
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '#dieu-2' }))
  const shell = document.createElement('div')
  shell.className = 'site-layout'
  shell.innerHTML = '<header class="site-header"></header>'
  document.body.append(shell)
  let jumped = false
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const top = this.dataset.spyAnchor === 'dieu-1' ? -500 : this.dataset.spyAnchor === 'dieu-2' ? (jumped ? 73 : 1000) : 0
    return { top, bottom: top + 20, height: this.classList.contains('site-header') ? 73 : 0 } as DOMRect
  })
  vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(function (this: HTMLElement) { if (this.dataset.anchor === 'dieu-2') jumped = true })
  class Observer { observe() {} unobserve() {} disconnect() {} }
  vi.stubGlobal('IntersectionObserver', Observer)
  const wrapper = mount(Reader, { attachTo: shell })
  await flushPromises()
  expect(wrapper.get('.reader-toc-desktop [aria-current="location"]').text()).toBe('Điều 2')
  wrapper.unmount()
  shell.remove()
})
