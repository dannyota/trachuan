// @vitest-environment happy-dom
import { computed, ref, onUnmounted } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import CopyButton from '../app/components/CopyButton.vue'

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('onUnmounted', onUnmounted)
})
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })

it('shows a check and copied label for two seconds after copying', async () => {
  vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  const wrapper = mount(CopyButton, { props: { text: 'citation', label: 'Sao chép trích dẫn' } })
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.get('button').text()).toBe('Đã sao chép')
  expect(wrapper.get('button svg[data-icon="check"]').exists()).toBe(true)
  await vi.advanceTimersByTimeAsync(2000)
  expect(wrapper.get('button').text()).toBe('Sao chép trích dẫn')
  wrapper.unmount()
})

it('announces the existing failure message when copying fails', async () => {
  vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('refused')) } })
  const wrapper = mount(CopyButton, { props: { text: 'citation', label: 'Sao chép trích dẫn' } })
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.get('[aria-live]').text()).toBe('Đã xảy ra lỗi. Vui lòng thử lại.')
  expect(wrapper.get('[aria-live]').classes()).toContain('visually-hidden')
  expect(wrapper.get('button').text()).toBe('Chưa sao chép được')
  expect(wrapper.get('button svg[data-icon="alert"]').exists()).toBe(true)
  await vi.advanceTimersByTimeAsync(2000)
  expect(wrapper.get('button').text()).toBe('Sao chép trích dẫn')
  wrapper.unmount()
})
