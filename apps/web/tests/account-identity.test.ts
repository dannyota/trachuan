// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, defineComponent, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { createApi } from '../app/api/client'
import { accountActions } from '../app/api/account-actions'
import { useSessionStore } from '../app/stores/session'
import Account from '../app/pages/account.vue'
import AccountDeleteDialog from '../app/components/AccountDeleteDialog.vue'
import TokenCreateDialog from '../app/components/TokenCreateDialog.vue'

vi.mock('vue-router', () => ({ onBeforeRouteLeave: vi.fn() }))
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

function setup(rejected = false) {
  setActivePinia(createPinia())
  const session = useSessionStore()
  session.$patch({ account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'token-a', status: 'signed-in' })
  const sent: string[] = []
  const api = createApi({ origin: 'http://localhost', session, fetch: async input => {
    const request = input as Request
    if (request.method === 'GET') {
      const path = new URL(request.url).pathname
      const data = path.endsWith('/reports') ? { reports: [] } : path === '/api/v1/me' ? { ...session.account, csrf_token: session.csrfToken } : []
      return new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' } })
    }
    sent.push(request.headers.get('x-csrf-token') || '')
    if (rejected) return new Response(JSON.stringify({ error: { code: 'csrf_rejected' } }), { status: 403, headers: { 'content-type': 'application/json' } })
    return new Response(null, { status: 204 })
  } })
  vi.stubGlobal('useApi', () => api)
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  return { api, session, sent }
}
function switchAccount(session: ReturnType<typeof useSessionStore>) {
  session.$patch({ account: { id: 2, email: 'other@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'token-b' })
}
for (const kind of ['delete', 'create'] as const) {
  it(`rejects ${kind} when the account changes after the dialog opens`, async () => {
    const { api, session, sent } = setup()
    const action = api.captureAction()
    const wrapper = kind === 'delete' ? mount(AccountDeleteDialog, { props: { email: 'test@example.invalid', action } }) : mount(TokenCreateDialog, { props: { action } })
    switchAccount(session)
    await wrapper.get('input').setValue(kind === 'delete' ? 'test@example.invalid' : 'test')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(sent).toEqual([])
    expect(wrapper.text()).toContain('Your account changed. Please review and try again.')
    wrapper.unmount()
  })
}
it('never sends the new account token for a revoke already started', async () => {
  const { api, session, sent } = setup(true)
  const pending = accountActions(api.client).revokeGrant(1)
  switchAccount(session)
  const result = await pending
  expect(result.error?.error.code).toBe('account_changed')
  expect(sent).not.toContain('token-b')
})

for (const kind of ['delete', 'create'] as const) {
  it(`captures ${kind} identity when the dialog is requested`, async () => {
    const { session } = setup()
    vi.stubGlobal('definePageMeta', () => {})
    const stub = defineComponent({ props: ['action', 'email'], template: '<div />' })
    const wrapper = mount(Account, { global: { stubs: { TokenCreateDialog: stub, AccountDeleteDialog: stub } } })
    const label = kind === 'delete' ? 'Delete account…' : 'Create token'
    const button = wrapper.findAll('button').find(item => item.text() === label)!
    ;(button.element as HTMLButtonElement).click()
    switchAccount(session)
    await nextTick()
    expect(wrapper.findComponent(stub).props('action')).toMatchObject({ accountId: 1, csrfToken: 'token-a' })
    wrapper.unmount()
  })
}
