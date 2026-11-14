// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })
it('joins parts after continuation and pins the release', async () => {
  const calls = setupReader([
    { data: { release: '20261111_1', item: { complete: false, next: 'token', units: [{ kind: 'provision', anchor: 'a', text: 'first ', part: 1, parts: 2 }] } } },
    { data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', anchor: 'a', text: 'second', part: 2, parts: 2 }] } } },
  ], { release: '20261111_1' })
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).not.toContain('first ')
  await wrapper.get('.reader-more button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('first second')
  expect(calls[1]).toMatchObject({ params: { query: { release: '20261111_1', next: 'token' } } })
})
it('keeps loaded text when paging fails', async () => {
  const calls = setupReader([{ data: { release: '20261111_1', item: { complete: false, next: 'token', units: [{ kind: 'provision', text: 'saved text' }] } } }, { error: { error: { code: 'not_found' } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  await wrapper.get('.reader-more button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('saved text')
  expect(wrapper.text()).toContain('not found')
  expect(calls).toHaveLength(2)
})
it('finds an anchor that contains a literal percent escape', async () => {
  setupReader([{ data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', anchor: '%22', text: 'escaped anchor' }] } } }])
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '#%22' }))
  const scroll = vi.fn()
  HTMLElement.prototype.scrollIntoView = scroll
  mount(Reader)
  await flushPromises()
  expect(scroll).toHaveBeenCalledOnce()
})

it('loads a related document when its route key changes', async () => {
  const calls = setupReader([
    { data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', text: 'first document' }] } } },
    { data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', text: 'second document' }] } } },
  ])
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  const wrapper = mount(Reader)
  await flushPromises()
  route.params.key = 'vn:vbpl:2'
  await flushPromises()
  expect(wrapper.text()).toContain('second document')
  expect(wrapper.text()).not.toContain('first document')
  expect(calls[1]).toMatchObject({ params: { path: { key: 'vn:vbpl:2' } } })
})
it('reloads the document when the release query changes', async () => {
  const calls = setupReader([
    { data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', text: 'old release' }] } } },
    { data: { release: '20261112_1', item: { complete: true, units: [{ kind: 'provision', text: 'new release' }] } } },
  ])
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: { release: '20261111_1' }, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  const wrapper = mount(Reader)
  await flushPromises()
  route.query.release = '20261112_1'
  await flushPromises()
  expect(wrapper.text()).toContain('new release')
  expect(calls[1]).toMatchObject({ params: { query: { release: '20261112_1' } } })
})
it('stops looking for an unknown anchor after 50 pages', async () => {
  const pages = Array.from({ length: 51 }, (_, i) => ({ data: { release: '20261111_1', item: { complete: false, next: `page${i + 1}`, units: [{ kind: 'provision', anchor: `a${i}`, text: 'text' }] } } }))
  const calls = setupReader(pages)
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '#missing' }))
  const top = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  mount(Reader)
  await vi.waitFor(() => expect(top).toHaveBeenCalledWith(0, 0))
  expect(calls).toHaveLength(50)
  top.mockRestore()
})
it('does not send an invalid release query', async () => {
  const calls = setupReader([{ data: { release: '20261111_1', item: { complete: true, units: [] } } }], { release: 'invalid' })
  mount(Reader)
  await flushPromises()
  expect(calls[0]).toMatchObject({ params: { query: {} } })
})
it('observes provision and clause units as pages load and removes old targets', async () => {
  setupReader([
    { data: { release: '20990101', item: { complete: false, next: 'next', units: [
      { kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', text: 'Synthetic article' },
      { kind: 'provision', anchor: 'dieu-1/khoan-1', label: 'Khoản 1', text: 'Synthetic clause' },
    ] } } },
    { data: { release: '20990101', item: { complete: true, units: [{ kind: 'provision', anchor: 'dieu-2', label: 'Điều 2', text: 'Synthetic article' }] } } },
    { data: { release: '20990101', item: { complete: true, units: [] } } },
  ])
  const route = reactive({ params: { key: 'vn:vbpl:1' }, query: {}, hash: '' })
  vi.stubGlobal('useRoute', () => route)
  const observers: Observer[] = []
  class Observer {
    targets = new Set<Element>()
    constructor() { observers.push(this) }
    observe(element: Element) { this.targets.add(element) }
    unobserve(element: Element) { this.targets.delete(element) }
    disconnect() { this.targets.clear() }
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  const originalRect = HTMLElement.prototype.getBoundingClientRect
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    return this.closest('.reader-more') ? { top: 5000, bottom: 5050 } as DOMRect : originalRect.call(this)
  })
  const wrapper = mount(Reader)
  await flushPromises()
  const heading = wrapper.get('[data-spy-anchor="dieu-1"]').element
  const article = wrapper.get('[data-anchor="dieu-1"]').element
  const clause = wrapper.get('[data-anchor="dieu-1/khoan-1"]').element
  const initial = observers.findLast(observer => observer.targets.has(heading))!
  expect(initial.targets.has(article)).toBe(true)
  expect(initial.targets.has(clause)).toBe(true)
  await wrapper.get('.reader-more button').trigger('click')
  await flushPromises()
  const nextHeading = wrapper.get('[data-spy-anchor="dieu-2"]').element
  const nextArticle = wrapper.get('[data-anchor="dieu-2"]').element
  const expanded = observers.findLast(observer => observer.targets.has(nextHeading))!
  expect(expanded.targets.has(article)).toBe(true)
  expect(expanded.targets.has(clause)).toBe(true)
  expect(expanded.targets.has(nextArticle)).toBe(true)
  route.params.key = 'vn:vbpl:2'
  await flushPromises()
  expect(expanded.targets.size).toBe(0)
  wrapper.unmount()
  expect(observers.every(observer => observer.targets.size === 0)).toBe(true)
})
