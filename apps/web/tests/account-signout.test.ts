// @vitest-environment happy-dom
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onMounted, ref } from 'vue'
import Account from '../app/pages/account.vue'
import { createApi } from '../app/api/client'
import { useSessionStore } from '../app/stores/session'

afterEach(() => vi.unstubAllGlobals())
it('clears the session and returns home after signing out', async () => {
  setActivePinia(createPinia())
  const session = useSessionStore()
  session.account = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }
  session.csrfToken = 'csrf'
  session.status = 'signed-in'
  const navigate = vi.fn()
  const calls: string[] = []
  const api = createApi({ origin: 'http://localhost', session, fetch: async (input) => {
    const request = input as Request
    const path = new URL(request.url).pathname
    calls.push(`${request.method} ${path} ${request.headers.get('x-csrf-token')}`)
    if (request.method === 'POST') return new Response(null, { status: 204 })
    const data = path.endsWith('/reports') ? { reports: [] } : path.endsWith('/sessions') ? [{ id: 1, created_at: '2026-01-01T00:00:00Z', last_seen_at: '2026-01-01T00:00:00Z', current: true }] : []
    return new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' } })
  } })
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('useApi', () => api)
  vi.stubGlobal('navigateTo', navigate)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  const wrapper = mount(Account, { global: { stubs: { TokenCreateDialog: true, AccountDeleteDialog: true } } })
  await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === 'Sign out')!.trigger('click')
  await flushPromises()
  expect(calls).toContain('POST /api/v1/auth/logout csrf')
  expect(session.$state).toEqual({ account: null, csrfToken: null, status: 'signed-out' })
  expect(navigate).toHaveBeenCalledWith('/')
})
