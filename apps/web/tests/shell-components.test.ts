// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import CopyButton from '../app/components/CopyButton.vue'
import SiteHeader from '../app/components/SiteHeader.vue'
import SealLogo from '../app/components/SealLogo.vue'

function setup() {
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onUnmounted', onUnmounted)
}
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); document.body.innerHTML = '' })

it('announces copy success and failure', async () => {
  setup()
  vi.useFakeTimers()
  const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'))
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  const wrapper = mount(CopyButton, { props: { text: 'synthetic', label: 'Copy' } })
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(writeText).toHaveBeenCalledWith('synthetic')
  expect(wrapper.get('[aria-live]').text()).toBe('Copied')
  vi.advanceTimersByTime(2000)
  await wrapper.vm.$nextTick()
  expect(wrapper.get('[aria-live]').text()).toBe('')
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(wrapper.get('[aria-live]').text()).toBeTruthy()
  expect(wrapper.text()).not.toContain('denied')
  wrapper.unmount()
})

it('closes the mobile menu on Escape and outside click and returns focus', async () => {
  setup()
  setActivePinia(createPinia())
  vi.stubGlobal('useRoute', () => ({ path: '/', fullPath: '/' }))
  const wrapper = mount(SiteHeader, { attachTo: document.body, global: { stubs: { NuxtLink: { template: '<a><slot /></a>' }, SealLogo: true } } })
  const button = wrapper.get('button[aria-controls]')
  await button.trigger('click')
  expect(button.attributes('aria-expanded')).toBe('true')
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  await wrapper.vm.$nextTick()
  expect(button.attributes('aria-expanded')).toBe('false')
  expect(document.activeElement).toBe(button.element)
  await button.trigger('click')
  document.body.click()
  await wrapper.vm.$nextTick()
  expect(button.attributes('aria-expanded')).toBe('false')
  expect(document.activeElement).toBe(button.element)
  wrapper.unmount()
})

it('uses the approved seal geometry', () => {
  const seal = mount(SealLogo)
  expect(seal.get('svg').attributes('viewBox')).toBe('0 0 120 120')
  expect(seal.findAll('circle').map(circle => [circle.attributes('cx'), circle.attributes('cy'), circle.attributes('r'), circle.attributes('stroke-width')])).toEqual([['60', '60', '52', '6'], ['60', '60', '40', '2']])
  expect(seal.get('text').attributes()).toMatchObject({ x: '60', y: '74', 'font-size': '40', 'font-weight': '700' })
  expect(seal.get('text').text()).toBe('TC')
})
