// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import createClient from 'openapi-fetch'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { createApi } from '../app/api/client'
import { useSessionStore } from '../app/stores/session'
import { useAccountLink } from '../app/composables/useAccountLink'

const token = 'synthetic-link-token'
const recovery = 'Sign in, then open the link from your mail again.'
const invalid = 'This link has expired or has already been used. Request a new link.'
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
function setup(code = '', status = 200, signedIn = true, lookupFailure = false, purpose = 'add_password') {
  setActivePinia(createPinia())
  const session = useSessionStore()
  const account = { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', password: false, google: true, totp: false, totp_enrollment: true }
  if (signedIn) session.$patch({ account, csrfToken: 'csrf', status: 'signed-in' })
  const requests: string[] = []
  const bodies: unknown[] = []
  const language = ref('en')
  const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {})
  const fetch: typeof globalThis.fetch = async input => {
    const request = input as Request
    const path = new URL(request.url).pathname
    requests.push(path)
    if (path === '/api/v1/me') return signedIn ? json({ ...account, csrf_token: 'csrf' }) : new Response(null, { status: 204 })
    if (path === '/api/v1/auth/link' && !lookupFailure) return json({ purpose, email: account.email })
    if (!lookupFailure) bodies.push(await request.json())
    if (status === 200) return path.endsWith('/reset/complete') ? new Response(null, { status: 204 }) : json({})
    return json({ error: { code, message: 'fixed' } }, status)
  }
  vi.stubGlobal('useState', () => language)
  vi.stubGlobal('useAuthApi', () => createClient({ baseUrl: 'http://localhost', fetch }))
  vi.stubGlobal('useApi', () => createApi({ origin: 'http://localhost', session, fetch }))
  let link!: ReturnType<typeof useAccountLink>
  const wrapper = mount(defineComponent({ setup() { link = useAccountLink(token); return () => null } }))
  return { wrapper, link, requests, assign, bodies, language, account }
}
it('discards a live add-password link without a session and asks the visitor to reopen their mail', async () => {
  const { wrapper, link, requests, assign } = setup('', 200, false)
  try {
    await flushPromises()
    expect(link.error.value).toBe(recovery)
    expect(link.link.value).toBeNull()
    expect(new URL(link.signInHref.value, 'http://localhost').searchParams.get('next')).toBe('/account')
    await link.save()
    expect(requests).not.toContain('/api/v1/me/password')
    expect(assign).not.toHaveBeenCalled()
  } finally { wrapper.unmount() }
})
for (const [code, status] of [['account_changed', 409], ['reauth_required', 403], ['unauthenticated', 401]] as const) {
  it(`discards an add-password token after ${code} without calling it expired`, async () => {
    const { wrapper, link, requests, assign } = setup(code, status)
    try {
      await flushPromises()
      link.password.value = 'synthetic new password'
      await link.save()
      expect(link.error.value).toBe(recovery)
      expect(link.link.value).toBeNull()
      expect(link.password.value).toBe('')
      const count = requests.length
      await link.save()
      expect(requests).toHaveLength(count)
      expect(assign).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })
}
it.each([['link_invalid', 409, invalid], ['unavailable', 503, 'Something went wrong. Please try again.']])('uses expired copy only for API link_invalid, received %s', async (code, status, expected) => {
  const { wrapper, link } = setup(String(code), Number(status), true, true)
  try { await flushPromises(); expect(link.error.value).toBe(expected) }
  finally { wrapper.unmount() }
})

for (const purpose of ['registration', 'reset']) {
  it(`retains a ${purpose} token only in the active ceremony while language changes`, async () => {
    const { wrapper, link, bodies, language, assign } = setup('', 200, true, false, purpose)
    try {
      await flushPromises()
      link.password.value = 'synthetic replacement password'
      language.value = 'vi'
      expect(link.labels.value.newPassword).toBe('Mật khẩu mới')
      await link.save()
      expect(bodies).toEqual([{ token, password: 'synthetic replacement password' }])
      expect(link.password.value).toBe('')
      expect(link.link.value).toBeNull()
      if (purpose === 'reset') expect(link.saved.value).toBe(true)
      else expect(assign).toHaveBeenCalledWith('/account')
    } finally { wrapper.unmount() }
  })
}
it('rejects an account change found by the session refresh before sending the token', async () => {
  const { wrapper, link, account, requests } = setup()
  try {
    await flushPromises()
    account.id = 2
    link.password.value = 'synthetic replacement password'
    await link.save()
    expect(link.error.value).toBe(recovery)
    expect(link.link.value).toBeNull()
    expect(requests).not.toContain('/api/v1/me/password')
  } finally { wrapper.unmount() }
})
