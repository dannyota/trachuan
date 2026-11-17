// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import FrameworkUnits from '../app/components/FrameworkUnits.vue'

it('shows one fixed line for a reduced control without publisher text', () => {
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  const wrapper = mount(FrameworkUnits, { props: { handle: 'fw:fw-alpha:2.1', units: [{ kind: 'head', head: { handle: 'fw:fw-alpha:2.1#A', citation: 'A', kind: 'control', access: 'licensed', text_withheld: true } }] }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.text()).toContain("The publisher's text needs access to read. This account does not have it.")
  expect(wrapper.text()).not.toContain('Synthetic publisher text')
  vi.unstubAllGlobals()
})

it('shows a missing title without substituting the publisher title', () => {
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  const wrapper = mount(FrameworkUnits, { props: { handle: 'fw:fw-alpha:2.1', units: [
    { kind: 'head', head: { handle: 'fw:fw-alpha:2.1#A', citation: 'A', kind: 'control', access: 'licensed', text_withheld: false } },
    { kind: 'title_original', text: 'Synthetic publisher title' },
  ] }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('h1').text()).toBe('A · control')
  expect(wrapper.text()).toContain('Synthetic publisher title')
  vi.unstubAllGlobals()
})

it('renders edges with no returned evidence without an evidence placeholder', () => {
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  const edge = { from: { handle: 'fw:fw-beta:1.0#A', citation: 'A', version: '1.0' }, to: { citation: 'B', version: '2.1', code: 'fw-alpha' }, relation: 'related' as const, source: 'Synthetic source', resolved: false }
  const wrapper = mount(FrameworkUnits, { props: { handle: 'fw:fw-beta:1.0', units: [{ kind: 'mapping', edge }] }, global: { stubs: { NuxtLink: true } } })
  const withheld = mount(FrameworkUnits, { props: { handle: 'fw:fw-beta:1.0', units: [{ kind: 'mapping', edge: structuredClone(edge) }] }, global: { stubs: { NuxtLink: true } } })
  expect(withheld.html()).toBe(wrapper.html())
  expect(wrapper.text()).toContain('Synthetic source')
  expect(wrapper.text().toLowerCase()).not.toContain('evidence')
  vi.unstubAllGlobals()
})

it.each([
  { handle: 'fw:fw-alpha:2.1#A', citation: 'A', kind: 'control', access: 'licensed' as const, text_withheld: true, title: 'Synthetic title' },
  { handle: 'fw:fw-beta:1.0#B', citation: 'B', kind: 'control', access: 'free' as const, text_withheld: false, status: 'active' },
])('places the control heading and clean metadata in the identity band', (head) => {
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  const wrapper = mount(FrameworkUnits, { props: { handle: head.handle, units: [{ kind: 'head', head }] }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('header.reader-identity h1').text()).toBe(head.title || `${head.citation} · ${head.kind}`)
  const meta = wrapper.get('header.reader-identity .fw-meta').text().trim()
  expect(meta).not.toMatch(/^[·\s]|[·\s]$/)
  expect(meta).not.toContain('· ·')
  vi.unstubAllGlobals()
})

it('places a version name in the identity band with clean metadata', () => {
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  const version = { code: 'fw-alpha', version: '2.1', name: 'Synthetic name', publisher: 'Synthetic publisher', access: 'licensed' as const, readable: false }
  const wrapper = mount(FrameworkUnits, { props: { handle: 'fw:fw-alpha:2.1', units: [{ kind: 'head', version }] }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('header.reader-identity h1').text()).toBe('Synthetic name')
  expect(wrapper.get('header.reader-identity .fw-meta').text().trim()).not.toMatch(/^[·\s]|[·\s]$/)
  vi.unstubAllGlobals()
})

it('uses closed and open locks for access and a locked notice for withheld text', () => {
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  const head = { handle: 'fw:fw-alpha:2.1#A', citation: 'A', kind: 'control', access: 'licensed' as const, text_withheld: true }
  const wrapper = mount(FrameworkUnits, { props: { handle: head.handle, units: [{ kind: 'head', head }] }, global: { stubs: { NuxtLink: true } } })
  expect(wrapper.get('.fw-access svg').classes()).toContain('lucide-lock')
  expect(wrapper.get('.notice.fw-withheld svg').classes()).toContain('lucide-lock')
  const free = { ...head, access: 'free' as const, text_withheld: false }
  const freePage = mount(FrameworkUnits, { props: { handle: head.handle, units: [{ kind: 'head', head: free }] }, global: { stubs: { NuxtLink: true } } })
  expect(freePage.get('.fw-access svg').classes()).toContain('lucide-lock-open')
  expect(freePage.find('.fw-withheld').exists()).toBe(false)
  vi.unstubAllGlobals()
})
