// @vitest-environment happy-dom
import { computed, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import ResultCard from '../app/components/ResultCard.vue'

beforeEach(() => {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
})
afterEach(() => vi.unstubAllGlobals())

it('pins all reader links by default and can link to the current release', async () => {
  const fragment = (anchor: string) => ({ identity: { document_key: 'vn:vbpl:1', anchor }, role: 'body', start: 0, end: 1, text_start: 0, text_end: 1, complete: true })
  const hit = { doc: 'vn:vbpl:1', handle: 'vn:vbpl:1#dieu-1', citation: 'Điều 1', complete: true, fragments: [fragment('dieu-1/khoan-1'), fragment('dieu-1/khoan-2')], warnings: [{ number: '2/2099', kind: 'repeals', exact: true, citation: 'Điều 1', in_release: true, key: 'vn:vbpl:2', source: 'vn:vbpl:2#dieu-2' }] }
  const link = { template: '<a :href="to"><slot /></a>', props: ['to'] }
  const wrapper = mount(ResultCard, { props: { hit, release: '20990101' }, global: { stubs: { NuxtLink: link } } })
  expect(wrapper.text()).not.toContain('Status as of')
  const hrefs = () => wrapper.findAll('a').map(a => a.attributes('href'))
  expect(hrefs()).toContain('/vn/documents/vn:vbpl:1?release=20990101#dieu-1')
  expect(hrefs()).toContain('/vn/documents/vn:vbpl:1?release=20990101#dieu-1/khoan-1')
  expect(hrefs()).toContain('/vn/documents/vn:vbpl:2?release=20990101#dieu-2')
  await wrapper.setProps({ currentRelease: true })
  expect(hrefs()).toContain('/vn/documents/vn:vbpl:1#dieu-1')
  expect(hrefs()).toContain('/vn/documents/vn:vbpl:1#dieu-1/khoan-1')
  expect(hrefs()).toContain('/vn/documents/vn:vbpl:2#dieu-2')
  expect(hrefs().every(href => !href?.includes('release='))).toBe(true)
  await wrapper.setProps({ currentRelease: false, release: undefined })
  expect(hrefs()).toHaveLength(0)
})
