// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import ReaderUnit from '../app/components/ReaderUnit.vue'
import { setupReader } from './reader-setup'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

it('updates the current TOC and compact label without rendering the page or its units', async () => {
  setupReader([{ data: { release: '20990101', item: { complete: true, units: [
    ...[1, 2].map(i => ({ kind: 'outline', anchor: `dieu-${i}`, label: `Điều ${i}` })),
    ...[1, 2].map(i => ({ kind: 'provision', anchor: `dieu-${i}`, label: `Điều ${i}`, text: 'Synthetic text' })),
  ] } } }])
  const shell = document.createElement('div')
  shell.className = 'site-layout'
  shell.innerHTML = '<header class="site-header"></header>'
  document.body.append(shell)
  let secondTop = 500
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const top = this.dataset.spyAnchor === 'dieu-1' ? 40 : this.dataset.spyAnchor === 'dieu-2' ? secondTop : 0
    return { top, bottom: top + 20, height: this.classList.contains('site-header') ? 73 : 0 } as DOMRect
  })
  const frames: FrameRequestCallback[] = []
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.push(callback); return frames.length })
  vi.stubGlobal('cancelAnimationFrame', () => {})
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
  const pageRender = vi.spyOn(Reader, 'render')
  const unitRender = vi.spyOn(ReaderUnit, 'render')
  const wrapper = mount(Reader, { attachTo: shell })
  await flushPromises()
  expect(wrapper.get('.reader-toc-desktop [aria-current="location"]').text()).toBe('Điều 1')
  expect(wrapper.get('.reader-compact span').text()).toBe('Điều 1')
  const heading = wrapper.get('[data-spy-anchor="dieu-2"]').element
  const headingObserver = observers.findLast(observer => observer.targets.has(heading))!
  pageRender.mockClear()
  unitRender.mockClear()
  secondTop = 70
  headingObserver.notify()
  frames.shift()!(0)
  await flushPromises()
  expect(wrapper.get('.reader-toc-desktop [aria-current="location"]').text()).toBe('Điều 2')
  expect(wrapper.get('.reader-sheet [aria-current="location"]').text()).toBe('Điều 2')
  expect(wrapper.get('.reader-compact span').text()).toBe('Điều 2')
  expect(pageRender).not.toHaveBeenCalled()
  expect(unitRender).not.toHaveBeenCalled()
  wrapper.unmount()
  shell.remove()
})
