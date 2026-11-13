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
it('keeps the search return link while the first request is pending or fails', async () => {
  setupReader([])
  let finish!: (value: unknown) => void
  vi.stubGlobal('useApi', () => ({ client: { GET: () => new Promise(resolve => { finish = resolve }) } }))
  const wrapper = mount(Reader)
  expect(wrapper.find('.reader-back-mobile a[href="/vn/search"]').exists()).toBe(true)
  finish({ error: { error: { code: 'unavailable' } } })
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('unavailable')
  expect(wrapper.find('.reader-back-mobile a[href="/vn/search"]').exists()).toBe(true)
})
