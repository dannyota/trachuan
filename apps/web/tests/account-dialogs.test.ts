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
