// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import HomeHero from '../app/components/HomeHero.vue'
import { useSessionStore } from '../app/stores/session'
import { useSearchDraftStore } from '../app/stores/search-draft'

const navigate = vi.fn()
beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('navigateTo', navigate)
})
afterEach(() => { vi.unstubAllGlobals(); navigate.mockReset() })

it('fills a suggestion without submitting and sends signed-in searches through the draft', async () => {
  useSessionStore().status = 'signed-in'
  const wrapper = mount(HomeHero)
  await wrapper.find('.home-suggestions button').trigger('click')
  expect((wrapper.get('#home-query').element as HTMLInputElement).value).toBe('xác thực sinh trắc học khi chuyển tiền')
  expect(navigate).not.toHaveBeenCalled()
  await wrapper.get('input[type="checkbox"]').setValue(false)
  await wrapper.get('form').trigger('submit')
  expect(useSearchDraftStore().draft).toEqual({ query: 'xác thực sinh trắc học khi chuyển tiền', inForceOnly: false })
  expect(navigate).toHaveBeenCalledWith('/vn/search')
  expect(wrapper.text()).not.toContain('Cần đăng nhập Google')
})

it.each(['unknown', 'signed-out'] as const)('sends %s visitors to login without carrying their question', async (status) => {
  useSessionStore().status = status
  const wrapper = mount(HomeHero)
  await wrapper.get('#home-query').setValue('private question')
  expect(wrapper.text()).toContain('Cần đăng nhập Google để tra cứu.')
  await wrapper.get('form').trigger('submit')
  expect(useSearchDraftStore().draft).toBeNull()
  expect(navigate).toHaveBeenCalledWith('/api/v1/auth/google/start?next=%2Fvn%2Fsearch', { external: true })
})
