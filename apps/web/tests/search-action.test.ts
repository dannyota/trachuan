// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onBeforeUnmount, ref } from 'vue'
import Search from '../app/pages/vn/search.vue'
import { createApi } from '../app/api/client'

const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'content-type': 'application/json' } })
const page = { release: '20261111_1', abstain: false, hits: [{ doc: 'vn:vbpl:1' }] }
const expired = () => json({ error: { code: 'release_expired', message: 'expired' } }, 409)
function setup(fetch: typeof globalThis.fetch) {
  const session = { account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'token-a', status: 'signed-in' as const }
  const api = createApi({ origin: 'http://localhost', session, fetch })
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('useApi', () => api)
  const wrapper = mount(Search, { global: { stubs: { SearchResults: true } } })
  return { wrapper, session }
}
async function submit(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('textarea').setValue('fixture query')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}
async function more(wrapper: ReturnType<typeof mount>) {
  await wrapper.findAll('button').find(button => button.text() === 'Show more')!.trigger('click')
  await flushPromises()
}
afterEach(() => vi.unstubAllGlobals())
it('does not resend the query after the account changes during a pinned search', async () => {
  const calls: Request[] = []
  let finish!: (response: Response) => void
  const { wrapper, session } = setup(async input => {
    calls.push(input as Request)
    if (calls.length === 1) return json(page)
    if (calls.length === 2) return new Promise(resolve => { finish = resolve })
    return json(page)
  })
  await submit(wrapper)
  await more(wrapper)
  session.account = { ...session.account, id: 2 }
  session.csrfToken = 'token-b'
  finish(expired())
  await flushPromises()
  expect(calls).toHaveLength(2)
  expect(calls.map(call => call.headers.get('x-csrf-token'))).toEqual(['token-a', 'token-a'])
  wrapper.unmount()
})
it('aborts a pending search on unmount and drops a late expiration response', async () => {
  const calls: Request[] = []
  let finish!: (response: Response) => void
  const { wrapper } = setup(async input => {
    calls.push(input as Request)
    if (calls.length === 1) return json(page)
    if (calls.length === 2) return new Promise(resolve => { finish = resolve })
    return json(page)
  })
  await submit(wrapper)
  await more(wrapper)
  wrapper.unmount()
  expect(calls[1]!.signal.aborted).toBe(true)
  finish(expired())
  await flushPromises()
  expect(calls).toHaveLength(2)
})
it('restarts an expired pinned search once using the original capture', async () => {
  const calls: { body: Record<string, unknown>, csrf: string | null }[] = []
  const { wrapper, session } = setup(async input => {
    const request = input as Request
    calls.push({ body: await request.json(), csrf: request.headers.get('x-csrf-token') })
    return calls.length === 2 ? expired() : json(page)
  })
  await submit(wrapper)
  session.csrfToken = 'new-token'
  await more(wrapper)
  expect(calls).toHaveLength(3)
  expect(calls.map(call => call.csrf)).toEqual(['token-a', 'token-a', 'token-a'])
  expect(calls[1]!.body.release).toBe('20261111_1')
  expect(calls[2]!.body.release).toBeUndefined()
  wrapper.unmount()
})

it('does not refresh CSRF or retry after unmount during a pending request', async () => {
  const calls: Request[] = []
  let finish!: (response: Response) => void
  const { wrapper } = setup(async input => {
    calls.push(input as Request)
    return new Promise(resolve => { finish = resolve })
  })
  await submit(wrapper)
  wrapper.unmount()
  finish(json({ error: { code: 'csrf_rejected', message: 'rejected' } }, 403))
  await flushPromises()
  expect(calls).toHaveLength(1)
})
