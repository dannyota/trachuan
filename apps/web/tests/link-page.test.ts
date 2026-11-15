// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import createClient from 'openapi-fetch'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import LinkPage from '../app/pages/link.vue'
import { createApi } from '../app/api/client'
import { useSessionStore } from '../app/stores/session'

vi.mock('../app/composables/useAccountLink', async importOriginal => {
  const original = await importOriginal<typeof import('../app/composables/useAccountLink')>()
  return { useAccountLink: () => original.useAccountLink('synthetic-link-token') }
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

function setup(purpose: string, initialLanguage: 'vi' | 'en') {
  setActivePinia(createPinia())
  const session = useSessionStore()
  const account = { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', password: false, google: true, totp: false, totp_enrollment: true }
  const language = ref(initialLanguage)
  const head = vi.fn()
  const savedFlag = ref(false)
  const navigate = vi.fn()
  const json = (body: unknown) => new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } })
  const fetch: typeof globalThis.fetch = async input => {
    const path = new URL((input as Request).url).pathname
    if (path === '/api/v1/auth/link') return json({ purpose, email: account.email })
    if (path === '/api/v1/me') return json({ ...account, csrf_token: 'csrf' })
    return new Response(null, { status: 204 })
  }
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', (key: string) => key === 'language' ? language : savedFlag)
  vi.stubGlobal('navigateTo', navigate)
  vi.stubGlobal('useHead', head)
  vi.stubGlobal('useAuthApi', () => createClient({ baseUrl: 'http://localhost', fetch }))
  vi.stubGlobal('useApi', () => createApi({ origin: 'http://localhost', session, fetch }))
  const wrapper = mount(LinkPage, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  return { wrapper, head, language, savedFlag, navigate, session }
}

for (const purpose of ['registration', 'add_password', 'reset']) {
  for (const language of ['vi', 'en'] as const) {
    it(`titles the ${purpose} form in ${language}`, async () => {
      const { wrapper, head } = setup(purpose, language)
      try {
        await flushPromises()
        const expected = purpose === 'reset' ? (language === 'vi' ? 'Đặt lại mật khẩu' : 'Reset password') : (language === 'vi' ? 'Đặt mật khẩu' : 'Set your password')
        const title = expected === 'Set your password' ? 'Set password' : expected
        expect(head.mock.calls.at(-1)?.[0]().title).toBe(`${title} · Tra Chuẩn`)
        expect(wrapper.get('h1').text()).toBe(expected)
      } finally { wrapper.unmount() }
    })
  }
}

it('clears the session and navigates to login after reset', async () => {
  const { wrapper, savedFlag, navigate, session } = setup('reset', 'en')
  try {
    await flushPromises()
    await wrapper.get('input[type=password]').setValue('synthetic replacement password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(navigate).toHaveBeenCalledWith('/login')
    expect(savedFlag.value).toBe(true)
    expect(session.account).toBeNull()
    expect(session.csrfToken).toBeNull()
    expect(session.status).toBe('signed-out')
  } finally { wrapper.unmount() }
})

it.each(['registration', 'add_password', 'reset'])('associates the %s password with its account email', async purpose => {
  const { wrapper } = setup(purpose, 'en')
  try {
    await flushPromises()
    expect(wrapper.get('input[name=new-password]').attributes('maxlength')).toBeUndefined()
    const username = wrapper.get('form input[name=username]')
    expect(username.attributes('type')).toBe('email')
    expect(username.attributes('autocomplete')).toBe('username')
    expect(username.attributes('readonly')).toBeDefined()
    expect(username.element).toHaveProperty('value', 'test@example.invalid')
  } finally { wrapper.unmount() }
})
