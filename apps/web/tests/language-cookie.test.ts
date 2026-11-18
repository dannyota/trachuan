// @vitest-environment happy-dom
import { createPinia, setActivePinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import SiteHeader from '../app/components/SiteHeader.vue'
import SummaryHeader from '../app/components/SummaryHeader.vue'
import { saveLanguage } from '../app/utils/language-cookie'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers() })

function mountHeader(component: typeof SiteHeader | typeof SummaryHeader) {
  setActivePinia(createPinia())
  vi.stubGlobal('useState', () => ref('vi'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onUnmounted', onUnmounted)
  vi.stubGlobal('useRoute', () => ({ path: '/', fullPath: '/' }))
  return mount(component, { global: { stubs: { NuxtLink: { template: '<a><slot /></a>' }, SealLogo: true } } })
}

it.each([['SiteHeader', SiteHeader], ['SummaryHeader', SummaryHeader]])('%s does not reload until the cookie store write completes', async (_name, component) => {
  let finish: () => void = () => {}
  const set = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
  vi.stubGlobal('cookieStore', { set })
  const reload = vi.fn()
  vi.spyOn(window, 'location', 'get').mockReturnValue({ ...window.location, reload } as Location)
  const wrapper = mountHeader(component)
  await wrapper.find('.language-button').trigger('click')
  await flushPromises()
  expect(set).toHaveBeenCalledWith(expect.objectContaining({ name: 'lang', value: 'en', path: '/', sameSite: 'lax' }))
  expect(reload).not.toHaveBeenCalled()
  finish()
  await flushPromises()
  expect(reload).toHaveBeenCalledOnce()
  wrapper.unmount()
})

it('sets the expiry one year ahead', async () => {
  const set = vi.fn(async () => {})
  vi.stubGlobal('cookieStore', { set })
  const before = Date.now()
  await saveLanguage('vi')
  const { expires } = (set.mock.calls[0] as unknown as [{ expires: number }])[0]
  expect(expires).toBeGreaterThanOrEqual(before + 31536000000)
  expect(expires).toBeLessThanOrEqual(Date.now() + 31536000000)
})

it('falls back to document.cookie when the cookie store rejects', async () => {
  vi.stubGlobal('cookieStore', { set: vi.fn(async () => { throw new Error('denied') }) })
  vi.spyOn(window, 'location', 'get').mockReturnValue({ hostname: 'example.test', protocol: 'https:' } as Location)
  const write = vi.spyOn(document, 'cookie', 'set')
  await saveLanguage('en')
  expect(write).toHaveBeenCalledWith('lang=en; Path=/; Max-Age=31536000; SameSite=Lax; Secure')
})

it('falls back when the cookie store write is still pending after one second', async () => {
  vi.useFakeTimers()
  vi.stubGlobal('cookieStore', { set: vi.fn(() => new Promise<void>(() => {})) })
  const write = vi.spyOn(document, 'cookie', 'set')
  const done = vi.fn()
  const pending = saveLanguage('en').then(done)
  await vi.advanceTimersByTimeAsync(999)
  expect(done).not.toHaveBeenCalled()
  expect(write).not.toHaveBeenCalled()
  await vi.advanceTimersByTimeAsync(1)
  await pending
  expect(write).toHaveBeenCalledOnce()
})

it('writes document.cookie without a cookie store and omits Secure on http loopback', async () => {
  vi.stubGlobal('cookieStore', undefined)
  const write = vi.spyOn(document, 'cookie', 'set')
  await saveLanguage('vi')
  expect(write).toHaveBeenCalledWith('lang=vi; Path=/; Max-Age=31536000; SameSite=Lax')
  vi.spyOn(window, 'location', 'get').mockReturnValue({ hostname: 'example.test', protocol: 'https:' } as Location)
  await saveLanguage('en')
  expect(write).toHaveBeenLastCalledWith('lang=en; Path=/; Max-Age=31536000; SameSite=Lax; Secure')
})

it('reads document.cookie back after the write', async () => {
  vi.stubGlobal('cookieStore', undefined)
  const order: string[] = []
  vi.spyOn(document, 'cookie', 'set').mockImplementation(() => { order.push('write') })
  let reads = 0
  vi.spyOn(document, 'cookie', 'get').mockImplementation(() => { order.push('read'); return ++reads < 3 ? 'other=1' : 'other=1; lang=en' })
  await saveLanguage('en')
  expect(order).toEqual(['write', 'read', 'read', 'read'])
})

it('stops reading after 50 reads when cookies are blocked', async () => {
  vi.stubGlobal('cookieStore', undefined)
  vi.spyOn(document, 'cookie', 'set').mockImplementation(() => {})
  const read = vi.spyOn(document, 'cookie', 'get').mockReturnValue('')
  await expect(saveLanguage('en')).resolves.toBeUndefined()
  expect(read).toHaveBeenCalledTimes(50)
})
