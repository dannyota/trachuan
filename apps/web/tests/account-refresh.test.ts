// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { createApi } from '../app/api/client'
import { useSessionStore } from '../app/stores/session'
import Account from '../app/pages/account.vue'
import AccountSecurity from '../app/components/AccountSecurity.vue'
import AccountPassword from '../app/components/AccountPassword.vue'
import AccountAuthenticator from '../app/components/AccountAuthenticator.vue'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } })
function setup(kind: string, failList = false) {
  setActivePinia(createPinia())
  const session = useSessionStore()
  session.$patch({ account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', password: true, google: false, totp: kind !== 'confirm', totp_enrollment: true }, csrfToken: 'old', status: 'signed-in' })
  let changed = false
  const unavailable = vi.fn()
  const fetch: typeof globalThis.fetch = async input => {
    const request = input as Request
    const path = new URL(request.url).pathname
    if (path.endsWith('/setup')) return json({ secret: 'synthetic-secret', otpauth_uri: 'otpauth://totp/synthetic' })
    if (request.method !== 'GET') { changed = true; return json(path.endsWith('/confirm') || path.endsWith('/recovery-codes') ? { recovery_codes: ['ABCDEFGHIJKLMNOPQRSTUVWXYZ'] } : {}) }
    if (path === '/api/v1/me') return json({ ...session.account, csrf_token: changed ? 'new' : 'old' })
    if (path.endsWith('/sessions')) return json(changed ? [{ id: 3, current: true }] : [{ id: 1, current: true }, { id: 2, current: false }])
    if (path.endsWith('/tokens')) return json([{ id: 1, name: 'Old token', revoked_at: changed ? '2026-01-02T00:00:00Z' : null }])
    if (path.endsWith('/grants')) return changed && failList ? json({ error: { code: 'unavailable' } }, 503) : json(changed ? [] : [{ id: 1, client_name: 'Old grant' }])
    return json({ reports: [] })
  }
  vi.stubGlobal('useApi', (options: { localUnavailable?: boolean } = {}) => createApi({ origin: 'http://localhost', session, fetch, onUnavailable: options.localUnavailable ? undefined : unavailable }))
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onUnmounted', onUnmounted)
  const wrapper = mount(Account, { global: { components: { AccountSecurity }, stubs: { ReportsSection: true, TokenCreateDialog: true, AccountDeleteDialog: true } } })
  return { wrapper, session, unavailable }
}
for (const kind of ['password', 'confirm', 'remove', 'recovery']) {
  it(`refreshes credential lists after ${kind} and preserves returned recovery codes`, async () => {
    const { wrapper, session } = setup(kind)
    try {
      await flushPromises()
      expect(wrapper.text()).toContain('Old grant')
      const security = wrapper.findComponent(AccountAuthenticator)
      if (kind === 'password') {
        const password = wrapper.findComponent(AccountPassword)
        await password.get('input[autocomplete=current-password]').setValue('synthetic current password')
        await password.get('input[autocomplete=new-password]').setValue('synthetic replacement password')
        await password.get('form').trigger('submit')
      } else {
        if (kind === 'confirm') { await security.get('button').trigger('click'); await flushPromises() }
        await security.get('input').setValue('123456')
        if (kind === 'recovery') await security.findAll('button').find(button => button.text() === 'Replace recovery codes')!.trigger('click')
        else await security.get('form').trigger('submit')
      }
      await flushPromises()
      const sessions = wrapper.findAll('.account-card').find(section => section.text().startsWith('Sessions'))!
      expect(sessions.findAll('.account-row')).toHaveLength(1)
      expect(wrapper.text()).not.toContain('Old grant')
      expect(wrapper.findAll('.account-row').find(row => row.text().includes('Old token'))!.text()).toContain('Revoked')
      expect(session.csrfToken).toBe('new')
      if (kind === 'confirm' || kind === 'recovery') expect(wrapper.get('.recovery-codes').text()).toContain('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
    } finally { wrapper.unmount() }
  })
}
it('keeps one-time codes visible when refreshing a credential list fails', async () => {
  const { wrapper, unavailable } = setup('recovery', true)
  try {
    await flushPromises()
    const security = wrapper.findComponent(AccountAuthenticator)
    await security.get('input').setValue('123456')
    await security.findAll('button').find(button => button.text() === 'Replace recovery codes')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('.recovery-codes').text()).toContain('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
    expect(wrapper.findAll('[role=alert]').length).toBeGreaterThan(0)
    expect(unavailable).not.toHaveBeenCalled()
  } finally { wrapper.unmount() }
})
