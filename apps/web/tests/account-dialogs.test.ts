// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { computed, onBeforeUnmount, onMounted, onUnmounted, ref } from 'vue'
import TokenCreateDialog from '../app/components/TokenCreateDialog.vue'
import AccountDeleteDialog from '../app/components/AccountDeleteDialog.vue'

vi.mock('vue-router', () => ({ onBeforeRouteLeave: vi.fn() }))
const action = { accountId: 1, csrfToken: 'csrf', bound: false }

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('onUnmounted', onUnmounted)
  vi.stubGlobal('useApi', () => ({ forAction: () => ({ POST: async () => ({ data: { token: 'fixture-secret' } }) }) }))
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('opens both account dialogs as modals', () => {
  const showModal = vi.spyOn(HTMLDialogElement.prototype, 'showModal')
  const token = mount(TokenCreateDialog, { props: { action } })
  const deletion = mount(AccountDeleteDialog, { props: { email: 'test@example.invalid', action } })
  expect(showModal).toHaveBeenCalledTimes(2)
  token.unmount()
  deletion.unmount()
})

it('clears the created token when the dialog is cancelled', async () => {
  const wrapper = mount(TokenCreateDialog, { props: { action } })
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect((wrapper.get('input.token-value[readonly]').element as HTMLInputElement).value).toBe('fixture-secret')
  await wrapper.get('dialog').trigger('cancel')
  expect(wrapper.find('input.token-value').exists()).toBe(false)
  expect(wrapper.emitted('close')).toHaveLength(1)
  wrapper.unmount()
})

it('clears a token when the page is hidden', async () => {
  const wrapper = mount(TokenCreateDialog, { props: { action } })
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect((wrapper.get('input.token-value[readonly]').element as HTMLInputElement).value).toBe('fixture-secret')
  window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
  await flushPromises()
  expect(wrapper.find('input.token-value').exists()).toBe(false)
  wrapper.unmount()
})

it('ignores a pending token creation after the page is hidden', async () => {
  let resolve!: (value: { data: { token: string } }) => void
  vi.stubGlobal('useApi', () => ({ forAction: () => ({ POST: () => new Promise((done) => { resolve = done }) }) }))
  const wrapper = mount(TokenCreateDialog, { props: { action } })
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
  resolve({ data: { token: 'fixture-secret' } })
  await flushPromises()
  expect(wrapper.find('input.token-value').exists()).toBe(false)
  expect(wrapper.emitted('created')).toBeUndefined()
  wrapper.unmount()
})

it('clears a token on a persisted page restoration', async () => {
  const wrapper = mount(TokenCreateDialog, { props: { action } })
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  const event = new Event('pageshow')
  Object.defineProperty(event, 'persisted', { value: true })
  window.dispatchEvent(event)
  await flushPromises()
  expect(wrapper.find('input.token-value').exists()).toBe(false)
  wrapper.unmount()
})

it('requires email confirmation again when the account changes', async () => {
  vi.stubGlobal('useApi', () => ({ forAction: () => ({ DELETE: async () => ({ error: { error: { code: 'account_changed' } } }) }) }))
  const wrapper = mount(AccountDeleteDialog, { props: { email: 'test@example.invalid', action } })
  await wrapper.get('input').setValue('test@example.invalid')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
  expect(wrapper.text()).toContain('Your account changed. Please review and try again.')
  await wrapper.get('input').setValue('test@example.invalid')
  expect(wrapper.get('button[type=submit]').attributes('disabled')).toBeDefined()
  wrapper.unmount()
})


it.each([
  ['token', TokenCreateDialog, { action }],
  ['deletion', AccountDeleteDialog, { email: 'test@example.invalid', action }],
])('wraps focus at both ends of the %s dialog', async (_name, component, props) => {
  const wrapper = mount(component, { props, attachTo: document.body })
  const first = wrapper.get('input').element as HTMLElement
  const last = wrapper.findAll('button').at(-1)!.element as HTMLElement
  last.focus()
  const forward = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
  last.dispatchEvent(forward)
  expect(forward.defaultPrevented).toBe(true)
  expect(document.activeElement).toBe(first)
  const backward = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true })
  first.dispatchEvent(backward)
  expect(backward.defaultPrevented).toBe(true)
  expect(document.activeElement).toBe(last)
  wrapper.unmount()
})

it.each([
  ['token', TokenCreateDialog, { action }],
  ['deletion', AccountDeleteDialog, { email: 'test@example.invalid', action }],
])('returns focus to the opener when the %s dialog closes', async (_name, component, props) => {
  const opener = document.createElement('button')
  document.body.append(opener)
  opener.focus()
  const wrapper = mount(component, { props, attachTo: document.body })
  ;(wrapper.get('input').element as HTMLElement).focus()
  await wrapper.findAll('button').at(-1)!.trigger('click')
  expect(document.activeElement).toBe(opener)
  wrapper.unmount()
  opener.remove()
})

it('focuses and selects the token when it appears', async () => {
  const wrapper = mount(TokenCreateDialog, { props: { action }, attachTo: document.body })
  await wrapper.get('input').setValue('test')
  ;(wrapper.get('button[type=submit]').element as HTMLElement).focus()
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  const input = wrapper.get('input.token-value[readonly]').element as HTMLInputElement
  expect(document.activeElement).toBe(input)
  expect(input.selectionStart).toBe(0)
  expect(input.selectionEnd).toBe(input.value.length)
  wrapper.unmount()
})

it('wraps the controls shown after token creation', async () => {
  const wrapper = mount(TokenCreateDialog, { props: { action }, attachTo: document.body })
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  const [copy, close] = wrapper.findAll('button').map(button => button.element as HTMLElement)
  const input = wrapper.get('input.token-value[readonly]').element as HTMLElement
  expect(copy!.textContent).toBe('Copy')
  expect(close!.textContent).toBe('Close')
  expect(Array.from(wrapper.get('dialog').element.querySelectorAll('input, button'))).toEqual([input, copy, close])
  input.focus()
  const backward = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true })
  input.dispatchEvent(backward)
  expect(backward.defaultPrevented).toBe(true)
  expect(document.activeElement).toBe(close)
  const forward = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
  close!.dispatchEvent(forward)
  expect(forward.defaultPrevented).toBe(true)
  expect(document.activeElement).toBe(input)
  wrapper.unmount()
})

it.each([
  ['Create token', TokenCreateDialog, { action }],
  ['Delete account', AccountDeleteDialog, { email: 'test@example.invalid', action }],
])('names the %s dialog from its heading', (title, component, props) => {
  const wrapper = mount(component, { props })
  const id = wrapper.get('dialog').attributes('aria-labelledby')
  expect(id).toBeTruthy()
  expect(wrapper.get('h2').attributes('id')).toBe(id)
  expect(wrapper.get('h2').text()).toBe(title)
  wrapper.unmount()
})

it('asks for password reauth before retrying token creation', async () => {
  const { useSessionStore } = await import('../app/stores/session')
  useSessionStore().$patch({ account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', password: true, google: false, totp: false, totp_enrollment: true }, status: 'signed-in', csrfToken: 'csrf' })
  let attempts = 0
  vi.stubGlobal('useApi', () => ({ forAction: () => ({ POST: async (path: string) => {
    if (path.endsWith('/reauth')) return { response: { status: 204 } }
    attempts++
    return attempts === 1 ? { error: { error: { code: 'reauth_required' } }, response: { status: 403 } } : { data: { token: 'fixture-secret' } }
  } }) }))
  const wrapper = mount(TokenCreateDialog, { props: { action } })
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.text()).toContain('Confirm your password to continue')
  await wrapper.get('input[type=password]').setValue('synthetic password')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(attempts).toBe(2)
  expect((wrapper.get('input.token-value[readonly]').element as HTMLInputElement).value).toBe('fixture-secret')
  wrapper.unmount()
})
