// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import TokenCreateDialog from '../app/components/TokenCreateDialog.vue'
import AccountDeleteDialog from '../app/components/AccountDeleteDialog.vue'

vi.mock('vue-router', () => ({ onBeforeRouteLeave: vi.fn() }))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('useApi', () => ({ client: { POST: async () => ({ data: { token: 'fixture-secret' } }) } }))
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('opens both account dialogs as modals', () => {
  const showModal = vi.spyOn(HTMLDialogElement.prototype, 'showModal')
  const token = mount(TokenCreateDialog)
  const deletion = mount(AccountDeleteDialog, { props: { email: 'test@example.invalid' } })
  expect(showModal).toHaveBeenCalledTimes(2)
  token.unmount()
  deletion.unmount()
})

it('clears the created token when the dialog is cancelled', async () => {
  const wrapper = mount(TokenCreateDialog)
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.text()).toContain('fixture-secret')
  await wrapper.get('dialog').trigger('cancel')
  expect(wrapper.text()).not.toContain('fixture-secret')
  expect(wrapper.emitted('close')).toHaveLength(1)
  wrapper.unmount()
})

it('clears a token when the page is hidden', async () => {
  const wrapper = mount(TokenCreateDialog)
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.text()).toContain('fixture-secret')
  window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
  await flushPromises()
  expect(wrapper.text()).not.toContain('fixture-secret')
  wrapper.unmount()
})

it('ignores a pending token creation after the page is hidden', async () => {
  let resolve!: (value: { data: { token: string } }) => void
  vi.stubGlobal('useApi', () => ({ client: { POST: () => new Promise((done) => { resolve = done }) } }))
  const wrapper = mount(TokenCreateDialog)
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
  resolve({ data: { token: 'fixture-secret' } })
  await flushPromises()
  expect(wrapper.text()).not.toContain('fixture-secret')
  expect(wrapper.emitted('created')).toBeUndefined()
  wrapper.unmount()
})

it('clears a token on a persisted page restoration', async () => {
  const wrapper = mount(TokenCreateDialog)
  await wrapper.get('input').setValue('test')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  const event = new Event('pageshow')
  Object.defineProperty(event, 'persisted', { value: true })
  window.dispatchEvent(event)
  await flushPromises()
  expect(wrapper.text()).not.toContain('fixture-secret')
  wrapper.unmount()
})

it('requires email confirmation again when the account changes', async () => {
  vi.stubGlobal('useApi', () => ({ client: { DELETE: async () => ({ error: { error: { code: 'account_changed' } } }) } }))
  const wrapper = mount(AccountDeleteDialog, { props: { email: 'test@example.invalid' } })
  await wrapper.get('input').setValue('test@example.invalid')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
  expect(wrapper.text()).toContain('Your account changed. Please review and try again.')
  await wrapper.get('input').setValue('test@example.invalid')
  expect(wrapper.get('button[type=submit]').attributes('disabled')).toBeDefined()
  wrapper.unmount()
})
