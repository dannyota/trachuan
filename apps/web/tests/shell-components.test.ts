// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import CopyButton from '../app/components/CopyButton.vue'
import SiteHeader from '../app/components/SiteHeader.vue'
import SealLogo from '../app/components/SealLogo.vue'
import { useSessionStore } from '../app/stores/session'

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
  expect(button.attributes('aria-label')).toBeTruthy()
  const icon = button.get('svg')
  expect(icon.attributes()).toMatchObject({ width: '22', height: '22', 'aria-hidden': 'true', stroke: 'currentColor' })
  expect(icon.findAll('line')).toHaveLength(3)
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

it('excludes the avatar initial from the account link accessible text', () => {
  setup()
  setActivePinia(createPinia())
  useSessionStore().$patch({ status: 'signed-in', account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' } })
  vi.stubGlobal('useRoute', () => ({ path: '/', fullPath: '/' }))
  const wrapper = mount(SiteHeader, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' }, SealLogo: true } } })
  const link = wrapper.get('.account-link')
  expect(link.get('.avatar').text()).toBe('T')
  const accessible = link.element.cloneNode(true) as HTMLElement
  accessible.querySelectorAll('[aria-hidden="true"]').forEach(element => element.remove())
  expect(accessible.textContent).toBe('Account')
  wrapper.unmount()
})

it('gives the mobile menu button a centered 44px target', () => {
  const css = readFileSync('app/assets/shell.css', 'utf8')
  const mobile = css.split('@media (max-width: 767px)')[1]!
  const button = mobile.match(/\.menu-button\s*\{([^}]*)\}/)?.[1]
  expect(button).toMatch(/min-width:\s*44px/)
  expect(button).toMatch(/min-height:\s*44px/)
  expect(button).toMatch(/justify-content:\s*center/)
  expect(button).toMatch(/align-items:\s*center/)
})

it('switches the link ceremony language without reloading its document', async () => {
  setup()
  setActivePinia(createPinia())
  const language = ref('en')
  vi.stubGlobal('useState', () => language)
  vi.stubGlobal('useRoute', () => ({ path: '/link', fullPath: '/link' }))
  const reload = vi.spyOn(location, 'reload').mockImplementation(() => {})
  const wrapper = mount(SiteHeader, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' }, SealLogo: true } } })
  try {
    await wrapper.get('.language-button').trigger('click')
    expect(language.value).toBe('vi')
    expect(wrapper.get('.language-button').text()).toBe('English')
    expect(document.cookie).toContain('lang=vi')
    expect(reload).not.toHaveBeenCalled()
  } finally { wrapper.unmount() }
})
