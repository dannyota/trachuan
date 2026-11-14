// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import ReaderToc from '../app/components/ReaderToc.vue'

afterEach(() => vi.unstubAllGlobals())
it.each(['dieu-8/khoan-2', 'dieu-8/khoan-2/diem-a'])('selects the enclosing article for %s', (current) => {
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', () => ref('vi'))
  const wrapper = mount(ReaderToc, { props: { current, units: [{ kind: 'outline', anchor: 'dieu-8', label: 'Điều 8' }, { kind: 'outline', anchor: 'dieu-9', label: 'Điều 9' }] } })
  expect(wrapper.findAll('[aria-current="location"]').map(node => node.text())).toEqual(['Điều 8'])
})
it('separates labels from titles without changing punctuation or case', () => {
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', () => ref('vi'))
  const wrapper = mount(ReaderToc, { props: { current: '', units: [
    { kind: 'outline', anchor: 'dieu-1', label: 'Điều 1', heading: 'Phạm vi điều chỉnh' },
    { kind: 'outline', anchor: 'dieu-2', label: 'Điều 2.', heading: 'TITLE' },
    { kind: 'outline', anchor: 'dieu-3', label: 'Điều 3', heading: '' },
  ] } })
  expect(wrapper.findAll('a').map(node => node.text())).toEqual(['Điều 1. Phạm vi điều chỉnh', 'Điều 2. TITLE', 'Điều 3'])
})
it('changes only the previous and current article when the current location changes', async () => {
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', () => ref('vi'))
  const units = Array.from({ length: 300 }, (_, i) => ({ kind: 'outline' as const, anchor: `dieu-${i + 1}`, label: `Điều ${i + 1}` }))
  const wrapper = mount(ReaderToc, { props: { current: 'dieu-1', units } })
  const records: MutationRecord[] = []
  const observer = new MutationObserver(changes => records.push(...changes))
  observer.observe(wrapper.element, { subtree: true, attributes: true, attributeFilter: ['aria-current'] })
  await wrapper.setProps({ current: 'dieu-2' })
  await Promise.resolve()
  expect(records.map(record => (record.target as Element).getAttribute('href'))).toEqual(['#dieu-1', '#dieu-2'])
  expect(wrapper.find('[aria-current="location"]').text()).toBe('Điều 2')
  observer.disconnect()
})
it('marks both the current chapter and article', () => {
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', () => ref('vi'))
  const wrapper = mount(ReaderToc, { props: { current: 'dieu-2', units: [
    { kind: 'outline', anchor: 'chuong-i', label: 'Chương I' },
    { kind: 'outline', anchor: 'dieu-1', label: 'Điều 1' },
    { kind: 'outline', anchor: 'dieu-2', label: 'Điều 2' },
  ] } })
  expect(wrapper.findAll('[aria-current="location"]').map(node => node.text())).toEqual(['Chương I', 'Điều 2'])
})
it('keeps a late selected entry inside the TOC scroll box', async () => {
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', () => ref('vi'))
  const container = document.createElement('div')
  document.body.append(container)
  const scrollbox: { current?: HTMLElement } = {}
  const originalRect = HTMLElement.prototype.getBoundingClientRect
  const rect = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this === scrollbox.current) return { top: 100, bottom: 300, height: 200 } as DOMRect
    if (this.dataset.tocAnchor) {
      const index = Number(this.dataset.tocAnchor.slice(5)) - 1
      const top = 100 + index * 20 - (scrollbox.current?.scrollTop || 0)
      return { top, bottom: top + 20, height: 20 } as DOMRect
    }
    return originalRect.call(this)
  })
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(200)
  const units = Array.from({ length: 300 }, (_, i) => ({ kind: 'outline' as const, anchor: `dieu-${i + 1}`, label: `Điều ${i + 1}` }))
  const wrapper = mount(ReaderToc, { attachTo: container, props: { current: 'dieu-1', units } })
  scrollbox.current = wrapper.element.parentElement!
  Object.defineProperty(scrollbox.current, 'clientHeight', { value: 200 })
  Object.defineProperty(scrollbox.current, 'scrollHeight', { value: 6000 })
  await wrapper.setProps({ current: 'dieu-290' })
  const selected = wrapper.get('[aria-current="location"]').element.getBoundingClientRect()
  expect(selected.top).toBeGreaterThanOrEqual(100)
  expect(selected.bottom).toBeLessThanOrEqual(300)
  wrapper.unmount()
  rect.mockRestore()
  vi.restoreAllMocks()
  container.remove()
})
