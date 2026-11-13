// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { createApi } from '../app/api/client'
import { createPinia, setActivePinia } from 'pinia'
import Search from '../app/pages/vn/search.vue'
import SearchNodePicker from '../app/components/SearchNodePicker.vue'

const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'content-type': 'application/json' } })
afterEach(() => vi.unstubAllGlobals())
it.each(['status', 'taxonomy'])('keeps search state after %s fails and retries locally', async (failing) => {
  let finish!: (response: Response) => void
  let failed = false
  const unavailable = ref(false)
  const session = { account: { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z' }, csrfToken: 'token', status: 'signed-in' as const }
  const fetcher: typeof fetch = async input => {
    const request = input as Request
    const url = new URL(request.url)
    if (url.pathname === '/api/v1/search') return json({ release: '20990101', abstain: false, hits: [{ citation: 'Fixture result', complete: true }] })
    if (!failed && ((failing === 'status' && url.pathname === '/api/v1/status') || (failing === 'taxonomy' && url.searchParams.get('node') === '1.1'))) {
      failed = true
      return new Promise(resolve => { finish = resolve })
    }
    if (url.pathname === '/api/v1/status') return json({ doc_types: [{ value: 'Type', count: 1 }], issuers: [] })
    return json({ nodes: url.searchParams.has('node') ? [{ code: '1.1.c1', name: 'Child' }] : [{ code: '1', name: 'Root', children: [{ code: '1.1', name: 'Parent' }] }], truncated: false })
  }
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('onMounted', onMounted)
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  vi.stubGlobal('useRoute', () => ({ query: {}, meta: { signedIn: true }, path: '/vn/search', fullPath: '/vn/search' }))
  vi.stubGlobal('useApi', (options: { localUnavailable?: boolean } = {}) => createApi({ origin: 'http://localhost', session, fetch: fetcher, onUnavailable: options.localUnavailable ? undefined : () => { unavailable.value = true } }))
  const Shell = defineComponent({ setup: () => () => unavailable.value ? h('p', 'global unavailable') : h(Search) })
  setActivePinia(createPinia())
  const wrapper = mount(Shell, { global: { components: { SearchNodePicker }, stubs: { NuxtLayout: { template: '<div><slot name="header" /><slot /></div>' }, SearchResults: { props: ['result'], template: '<div>{{ result.hits[0].citation }}</div>' } } } })
  await flushPromises()
  await wrapper.get('textarea').setValue('fixture query')
  await wrapper.get('input[type="date"]').setValue('2024-01-01')
  await wrapper.findAll('input[type="checkbox"]').find(input => input.element.parentElement?.textContent?.includes('Parent'))!.setValue(true)
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  if (failing === 'taxonomy') await wrapper.findAll('button').find(button => button.attributes('aria-label') === 'Show children' && button.element.parentElement?.textContent?.includes('Parent'))!.trigger('click')
  finish(json({ error: { code: 'unavailable' } }, 503))
  await flushPromises()
  expect(unavailable.value).toBe(false)
  expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('fixture query')
  expect((wrapper.get('input[type="date"]').element as HTMLInputElement).value).toBe('2024-01-01')
  expect(wrapper.text()).toContain('Parent ×')
  expect(wrapper.text()).toContain('Fixture result')
  expect(wrapper.text()).toContain('unavailable')
  await wrapper.findAll('button').find(button => button.text() === 'Retry')!.trigger('click')
  await flushPromises()
  expect(wrapper.text()).not.toContain('unavailable')
  expect(wrapper.text()).toContain(failing === 'taxonomy' ? 'Child' : 'Type')
  wrapper.unmount()
})
