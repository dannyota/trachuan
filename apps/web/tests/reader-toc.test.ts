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
