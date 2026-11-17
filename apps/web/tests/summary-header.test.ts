// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import SummaryHeader from '../app/components/SummaryHeader.vue'

afterEach(() => vi.unstubAllGlobals())

it('mounts its language and navigation controls without a session store or account probe', () => {
  const fetch = vi.fn()
  vi.stubGlobal('fetch', fetch)
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onUnmounted', onUnmounted)
  vi.stubGlobal('useRoute', () => ({ path: '/vn/documents/vn:vbpl:1/summary', fullPath: '/vn/documents/vn:vbpl:1/summary?ignored=value' }))
  const wrapper = mount(SummaryHeader, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' }, SealLogo: true } } })
  expect(wrapper.text()).toContain('Sign in')
  expect(wrapper.text()).toContain('Topics')
  expect(wrapper.text()).toContain('Tiếng Việt')
  expect(wrapper.findAll('a').find(link => link.text() === 'Sign in')?.attributes('href')).toBe('/login?next=%2Fvn%2Fdocuments%2Fvn%3Avbpl%3A1%2Fsummary')
  expect(wrapper.findAll('a').find(link => link.text() === 'Sign in')?.classes()).toEqual(['btn', 'btn-primary'])
  expect(fetch).not.toHaveBeenCalled()
  wrapper.unmount()
})
