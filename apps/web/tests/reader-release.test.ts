// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
it('offers the current release when the requested release expired', async () => {
  setupReader([{ error: { error: { code: 'release_expired' } } }], { release: '20261111_1' })
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).toContain('expired')
  expect(wrapper.find('a[href="/vn/documents/vn:vbpl:1"]').exists()).toBe(true)
})

it('requests an unsuffixed release', async () => {
  const calls = setupReader([{ data: { release: '20990101', item: { complete: true } } }], { release: '20990101' })
  mount(Reader, { global: { stubs: ['NuxtLink'] } })
  await flushPromises()
  expect(calls[0]).toMatchObject({ params: { query: { release: '20990101' } } })
})
