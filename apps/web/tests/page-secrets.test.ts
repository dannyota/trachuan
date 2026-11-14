// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { usePageSecrets } from '../app/composables/usePageSecrets'
import Factor from '../app/pages/login-factor.vue'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('clears secrets without navigating when the document is restored', () => {
  let secret = 'synthetic-secret'
  let signal!: AbortSignal
  const reload = vi.spyOn(window.location, 'reload').mockImplementation(() => {})
  const wrapper = mount(defineComponent({ setup() { signal = usePageSecrets(() => { secret = '' }).signal; return () => null } }))
  window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: false }))
  expect(reload).not.toHaveBeenCalled()
  window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
  expect(signal.aborted).toBe(true)
  expect(secret).toBe('')
  const restored = new Event('pageshow')
  Object.defineProperty(restored, 'persisted', { value: true })
  window.dispatchEvent(restored)
  expect(reload).not.toHaveBeenCalled()
  wrapper.unmount()
  reload.mockRestore()
})

it('renews cancellation for restored forms while keeping older requests aborted', () => {
  let secrets!: ReturnType<typeof usePageSecrets>
  const clear = vi.fn()
  const restore = vi.fn(() => { expect(secrets.signal.aborted).toBe(false) })
  const wrapper = mount(defineComponent({ setup() { secrets = usePageSecrets(clear, restore); return () => null } }))
  const original = secrets.signal
  window.dispatchEvent(new Event('pagehide'))
  const restored = new Event('pageshow')
  Object.defineProperty(restored, 'persisted', { value: true })
  window.dispatchEvent(restored)
  const current = secrets.signal
  expect(clear).toHaveBeenCalledTimes(1)
  expect(restore).toHaveBeenCalledTimes(1)
  expect(original.aborted).toBe(true)
  expect(current).not.toBe(original)
  expect(current.aborted).toBe(false)
  wrapper.unmount()
  expect(current.aborted).toBe(true)
  window.dispatchEvent(restored)
  expect(restore).toHaveBeenCalledTimes(1)
})

it('refreshes pending factor CSRF after restoration without reusing a cleared code', async () => {
  let reads = 0
  const post = vi.fn(async () => ({ data: { next: '/connect' } }))
  const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {})
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('usePageSecrets', usePageSecrets)
  vi.stubGlobal('useAuthApi', () => ({ GET: async () => ({ data: { csrf: `pending-${++reads}`, next: '/connect' } }), POST: post }))
  const wrapper = mount(Factor)
  try {
    await flushPromises()
    await wrapper.get('input').setValue('111111')
    window.dispatchEvent(new Event('pagehide'))
    const restored = new Event('pageshow')
    Object.defineProperty(restored, 'persisted', { value: true })
    window.dispatchEvent(restored)
    await flushPromises()
    expect(reads).toBe(2)
    expect(wrapper.get('input').element).toHaveProperty('value', '')
    await wrapper.get('input').setValue('222222')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/api/v1/auth/factor', expect.objectContaining({ params: { header: { Origin: window.location.origin, 'X-CSRF-Token': 'pending-2' } }, body: { code: '222222' } }))
    expect(assign).toHaveBeenCalledWith('/connect')
  } finally { wrapper.unmount() }
})
