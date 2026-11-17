// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, onBeforeUnmount, ref, Suspense } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import Search from '../app/pages/frameworks/search.vue'
import { createApi } from '../app/api/client'

it('does not send a search continuation after the account changes', async () => {
  const session = { account: { id: 1, email: 'synthetic@example.invalid', created_at: '2099-01-01T00:00:00Z' }, csrfToken: 'a', status: 'signed-in' as const }
  const posts: Request[] = []
  const json = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'content-type': 'application/json' } })
  const api = createApi({ origin: 'http://localhost', session, fetch: async input => {
    const request = input as Request
    if (request.method === 'GET') return json({ versions: {} })
    posts.push(request)
    return json({ release: '20990101', abstain: false, hits: [], next_cursor: 'next-a' })
  } })
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useFrameworkPage', () => ({ api, language: ref('en'), notFound: vi.fn() }))
  vi.stubGlobal('useAsyncData', async (_key: string, callback: () => Promise<unknown>) => ({ data: ref(await callback()), clear: vi.fn() }))
  const root = defineComponent({ render: () => h(Suspense, null, { default: () => h(Search) }) })
  const wrapper = mount(root, { global: { stubs: { NuxtLink: true, CorpusControl: true } } })
  await flushPromises()
  await wrapper.get('input').setValue('synthetic query')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  session.account = { ...session.account, id: 2 }
  session.csrfToken = 'b'
  await wrapper.get('button[type="button"]').trigger('click')
  await flushPromises()
  expect(posts).toHaveLength(1)
  expect(wrapper.get('[role="alert"]').text()).toContain('Your account changed')
  wrapper.unmount()
})
afterEach(() => vi.unstubAllGlobals())
