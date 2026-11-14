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
  const json = (body: unknown) => new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } })
  const fetch: typeof globalThis.fetch = async input => {
    const path = new URL((input as Request).url).pathname
    if (path === '/api/v1/auth/link') return json({ purpose, email: account.email })
    if (path === '/api/v1/me') return json({ ...account, csrf_token: 'csrf' })
    return new Response(null, { status: 204 })
  }
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('useState', () => language)
  vi.stubGlobal('useHead', head)
  vi.stubGlobal('useAuthApi', () => createClient({ baseUrl: 'http://localhost', fetch }))
  vi.stubGlobal('useApi', () => createApi({ origin: 'http://localhost', session, fetch }))
  const wrapper = mount(LinkPage, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  return { wrapper, head, language }
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

it('keeps the reset heading and title after saving and localizes them without restoring the form', async () => {
  const { wrapper, head, language } = setup('reset', 'en')
  try {
    await flushPromises()
    await wrapper.get('input[type=password]').setValue('synthetic replacement password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Reset password')
    expect(head.mock.calls.at(-1)?.[0]().title).toBe('Reset password · Tra Chuẩn')
    expect(wrapper.find('form').exists()).toBe(false)
    language.value = 'vi'
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Đặt lại mật khẩu')
    expect(head.mock.calls.at(-1)?.[0]().title).toBe('Đặt lại mật khẩu · Tra Chuẩn')
  } finally { wrapper.unmount() }
})

it.each(['registration', 'add_password', 'reset'])('associates the %s password with its account email', async purpose => {
  const { wrapper } = setup(purpose, 'en')
  try {
    await flushPromises()
    const username = wrapper.get('form input[name=username]')
    expect(username.attributes('type')).toBe('text')
    expect(username.attributes('autocomplete')).toBe('username')
    expect(username.classes()).toContain('sr-only')
    expect(username.element).toHaveProperty('value', 'test@example.invalid')
  } finally { wrapper.unmount() }
})
