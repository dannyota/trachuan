// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
it('renders legal text literally', async () => {
  setupReader([{ data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', anchor: 'a', text: '<Điều 1> & text' }] } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).toContain('<Điều 1> & text')
  expect(wrapper.find('điều').exists()).toBe(false)
})
