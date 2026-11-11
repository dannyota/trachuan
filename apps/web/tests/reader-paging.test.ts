// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import Reader from '../app/pages/vn/documents/[key].vue'
import { setupReader } from './reader-setup'

afterEach(() => vi.unstubAllGlobals())
it('joins parts after continuation and pins the release', async () => {
  const calls = setupReader([
    { data: { release: '20261111_1', item: { complete: false, next: 'token', units: [{ kind: 'provision', anchor: 'a', text: 'first ', part: 1, parts: 2 }] } } },
    { data: { release: '20261111_1', item: { complete: true, units: [{ kind: 'provision', anchor: 'a', text: 'second', part: 2, parts: 2 }] } } },
  ], { release: '20261111_1' })
  const wrapper = mount(Reader)
  await flushPromises()
  expect(wrapper.text()).not.toContain('first ')
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('first second')
  expect(calls[1]).toMatchObject({ params: { query: { release: '20261111_1', next: 'token' } } })
})
it('keeps loaded text when paging fails', async () => {
  const calls = setupReader([{ data: { release: '20261111_1', item: { complete: false, next: 'token', units: [{ kind: 'provision', text: 'saved text' }] } } }, { error: { error: { code: 'not_found' } } }])
  const wrapper = mount(Reader)
  await flushPromises()
  await wrapper.get('button').trigger('click')
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
