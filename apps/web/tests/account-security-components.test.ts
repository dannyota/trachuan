// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { computed, onUnmounted, ref } from 'vue'
import { createApi } from '../app/api/client'
import { useSessionStore } from '../app/stores/session'
import AccountAuthenticator from '../app/components/AccountAuthenticator.vue'
import AccountPassword from '../app/components/AccountPassword.vue'
import AccountSecurity from '../app/components/AccountSecurity.vue'
import AuthenticatorQr from '../app/components/AuthenticatorQr.vue'
import RecoveryCodes from '../app/components/RecoveryCodes.vue'

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('onUnmounted', onUnmounted)
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
function setup(handler: (request: Request) => Response | Promise<Response>, enrolled = false, enabled = true) {
  const session = useSessionStore()
  session.$patch({ account: { id: 1, email: 'synthetic@example.invalid', created_at: '2026-01-01T00:00:00Z', password: true, google: false, totp: enrolled, totp_enrollment: enabled }, csrfToken: 'old-csrf', status: 'signed-in' })
  const api = createApi({ origin: 'http://localhost', session, fetch: async input => new URL((input as Request).url).pathname === '/api/v1/me' ? json({ ...session.account, csrf_token: 'new-csrf' }) : handler(input as Request) })
  vi.stubGlobal('useApi', () => api)
  return session
}
it('renders a QR as SVG paths without embedding the URI', () => {
  const uri = 'otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP'
  const wrapper = mount(AuthenticatorQr, { props: { uri, label: 'Scan' } })
  expect(wrapper.get('svg').attributes('viewBox')).toMatch(/^0 0 \d+ \d+$/)
  expect(wrapper.get('path').attributes('d')).toContain('M4 4')
  expect(wrapper.html()).not.toContain(uri)
})
it('keeps removal and replacement available with enrollment disabled', () => {
  setup(() => json({}), true, false)
  const wrapper = mount(AccountAuthenticator)
  expect(wrapper.text()).not.toContain('Enable')
  expect(wrapper.text()).toContain('Disable')
  expect(wrapper.text()).toContain('Generate new recovery codes')
  wrapper.unmount()
})
it('clears setup and recovery codes on pagehide and refreshes CSRF after confirmation', async () => {
  const sent: string[] = []
  setup(request => {
    sent.push(request.headers.get('x-csrf-token')!)
    return json(request.url.endsWith('/setup') ? { secret: 'synthetic-secret', otpauth_uri: 'otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP' } : { recovery_codes: ['ABCDEFGHIJKLMNOPQRSTUVWXYZ'] })
  })
  const wrapper = mount(AccountAuthenticator)
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('synt heti c-se cret')
  await wrapper.get('input').setValue('123456')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.find('.auth-secret').exists()).toBe(false)
  expect(wrapper.text()).toContain('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
  expect(useSessionStore().csrfToken).toBe('new-csrf')
  expect(sent).toEqual(['old-csrf', 'old-csrf'])
  window.dispatchEvent(new Event('pagehide'))
  await flushPromises()
  expect(wrapper.find('.recovery-codes').exists()).toBe(false)
  wrapper.unmount()
})
it('ignores a setup response after leaving the page', async () => {
  let resolve!: (response: Response) => void
  setup(() => new Promise(done => { resolve = done }))
  const wrapper = mount(AccountAuthenticator)
  await wrapper.get('button').trigger('click')
  await flushPromises()
  window.dispatchEvent(new Event('pagehide'))
  resolve(json({ secret: 'synthetic-secret', otpauth_uri: 'otpauth://totp/synthetic' }))
  await flushPromises()
  expect(wrapper.text()).not.toContain('synt heti c-se cret')
  wrapper.unmount()
})
it('retries a password change with fresh primary proof and a requested factor', async () => {
  const bodies: unknown[] = []
  setup(async request => {
    if (request.url.endsWith('/reauth')) return new Response(null, { status: 204 })
    bodies.push(await request.json())
    return bodies.length === 1 ? json({ error: { code: 'reauth_required' }, factor: true }, 403) : json({})
  })
  const wrapper = mount(AccountPassword)
  await wrapper.get('button').trigger('click')
  await wrapper.get('input[autocomplete=current-password]').setValue('synthetic current password')
  await wrapper.get('input[autocomplete=new-password]').setValue('synthetic new password')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.text()).toContain('Confirm your password to continue')
  await wrapper.get('input[type=password]').setValue('synthetic current password')
  await wrapper.get('input[inputmode=numeric]').setValue('123456')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(bodies).toEqual([{ current: 'synthetic current password', password: 'synthetic new password' }, { current: 'synthetic current password', password: 'synthetic new password', code: '123456' }])
  expect(wrapper.text()).toContain('Security settings updated.')
  expect(wrapper.get('input[autocomplete=new-password]').element).toHaveProperty('value', '')
  wrapper.unmount()
})
it('downloads the same formatted codes offered for copying', async () => {
  const createObjectURL = vi.fn(() => 'blob:synthetic')
  vi.stubGlobal('URL', class extends URL { static createObjectURL = createObjectURL; static revokeObjectURL = vi.fn() })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  const wrapper = mount(RecoveryCodes, { props: { codes: ['ABCDEFGHIJKLMNOPQRSTUVWXYZ'] } })
  await wrapper.findAll('button').find(button => button.text() === 'Download recovery codes')!.trigger('click')
  const blob = createObjectURL.mock.calls[0]![0] as Blob
  expect(await blob.text()).toBe('ABCDE FGHIJ KLMNO PQRST UVWXY Z\n')
  expect(wrapper.get('pre').text()).toBe('ABCDE FGHIJ KLMNO PQRST UVWXY Z')
  wrapper.unmount()
})

it('rejects a pre-restore setup response and permits a fresh setup on the restored page', async () => {
  const reload = vi.spyOn(window.location, 'reload').mockImplementation(() => {})
  let finish!: (response: Response) => void
  const signals: AbortSignal[] = []
  setup(request => {
    signals.push(request.signal)
    if (signals.length === 1) return new Promise(resolve => { finish = resolve })
    return json({ secret: 'fresh-secret', otpauth_uri: 'otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP' })
  })
  const wrapper = mount(AccountAuthenticator)
  try {
    await wrapper.get('button').trigger('click')
    await flushPromises()
    window.dispatchEvent(new Event('pagehide'))
    const restored = new Event('pageshow')
    Object.defineProperty(restored, 'persisted', { value: true })
    window.dispatchEvent(restored)
    finish(json({ secret: 'stale-secret', otpauth_uri: 'otpauth://totp/synthetic' }))
    await flushPromises()
    expect(wrapper.text()).not.toContain('stal e-se cret')
    expect(signals[0]!.aborted).toBe(true)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(signals).toHaveLength(2)
    expect(signals[1]!.aborted).toBe(false)
    expect(wrapper.text()).toContain('fres h-se cret')
    expect(reload).not.toHaveBeenCalled()
  } finally { wrapper.unmount() }
})
it('shows the setup key in groups of four', async () => {
  setup(() => json({ secret: 'ABCDEFGHIJKLMNOP', otpauth_uri: 'otpauth://totp/synthetic?secret=ABCDEFGHIJKLMNOP' }))
  const wrapper = mount(AccountAuthenticator)
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.get('.auth-secret').text()).toContain('ABCD EFGH IJKL MNOP')
  wrapper.unmount()
})
it('shows password and two-step states from the account', () => {
  setup(() => json({}), true)
  const wrapper = mount(AccountSecurity)
  expect(wrapper.text()).toContain('Password: Set')
  expect(wrapper.text()).toContain('Two-step verification: On')
  wrapper.unmount()
})
it('shows unset and disabled states and copies the ungrouped setup key', async () => {
  const session = setup(() => json({ secret: 'ABCDEFGHIJKLMNOP', otpauth_uri: 'otpauth://totp/synthetic?secret=ABCDEFGHIJKLMNOP' }))
  session.account!.password = false
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  const wrapper = mount(AccountSecurity)
  expect(wrapper.text()).toContain('Password: Not set')
  expect(wrapper.text()).toContain('Two-step verification: Off')
  await wrapper.findComponent(AccountAuthenticator).get('button').trigger('click')
  await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === 'Copy key')!.trigger('click')
  expect(writeText).toHaveBeenCalledWith('ABCDEFGHIJKLMNOP')
  wrapper.unmount()
})
it.each([
  ['Generate new recovery codes', 'POST', '/api/v1/me/recovery-codes'],
  ['Disable', 'DELETE', '/api/v1/me/totp'],
])('submits the chosen %s operation from its form', async (action, method, path) => {
  const calls: string[] = []
  setup(request => {
    calls.push(`${request.method} ${new URL(request.url).pathname}`)
    return json(path.endsWith('recovery-codes') ? { recovery_codes: ['ABCDEFGHIJKLMNOPQRSTUVWXYZ'] } : {})
  }, true)
  const wrapper = mount(AccountAuthenticator)
  await wrapper.findAll('button').find(button => button.text() === action)!.trigger('click')
  await wrapper.get('input').setValue('123456')
  expect(wrapper.get('form button:not([type=button])').text()).toBe(action)
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(calls).toEqual([`${method} ${path}`])
  wrapper.unmount()
})
