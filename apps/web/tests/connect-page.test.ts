// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { computed, onUnmounted, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import Connect from '../app/pages/connect.vue'
import CopyButton from '../app/components/CopyButton.vue'

afterEach(() => vi.unstubAllGlobals())

it('keeps copy controls outside code boxes and copies the complete values', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onUnmounted', onUnmounted)
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { siteOrigin: 'https://example.invalid' } }))
  const wrapper = mount(Connect, { global: { components: { CopyButton }, stubs: { NuxtLink: { template: '<a><slot /></a>' } } } })
  const boxes = wrapper.findAll('.page-code')
  expect(boxes).toHaveLength(3)
  expect(wrapper.findAll('.page-code button')).toHaveLength(0)
  const buttons = wrapper.findAll('button')
  expect(buttons).toHaveLength(3)
  for (let i = 0; i < buttons.length; i++) {
    await buttons[i]!.trigger('click')
    expect(writeText).toHaveBeenLastCalledWith(boxes[i]!.text())
  }
  wrapper.unmount()
})
