// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, onBeforeUnmount, ref, Suspense } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import List from '../app/pages/frameworks/index.vue'
import Search from '../app/pages/frameworks/search.vue'
import Version from '../app/pages/frameworks/[code]/[version]/index.vue'
import Control from '../app/pages/frameworks/[code]/[version]/control.vue'
import FrameworkRead from '../app/components/FrameworkRead.vue'
import FrameworkGaps from '../app/components/FrameworkGaps.vue'
import FrameworkUnits from '../app/components/FrameworkUnits.vue'

const alpha = 'fw:fw-alpha:2.1'
const beta = 'fw:fw-beta:1.0'
const counts = { controls: 1, edges_resolved: 0, edges_unresolved: 0 }
function setup(mode: string, page: typeof List, unavailable = false, missing = false, replies: unknown[] = []) {
  const readable = mode === 'full'
  const versions = { [alpha]: { ...counts, access: 'licensed', readable }, ...(mode === 'reduced' ? {} : { [beta]: { ...counts, access: 'free', readable: true } }) }
  const framework = { code: 'fw-alpha', version: '2.1', name: 'Synthetic alpha', publisher: 'Synthetic publisher', access: 'licensed', readable }
  const betaFramework = { code: 'fw-beta', version: '1.0', name: 'Synthetic beta', publisher: 'Synthetic publisher', access: 'free', readable: true }
  const control = { handle: `${alpha}#A`, citation: 'A', kind: 'control', access: 'licensed', text_withheld: !readable }
  const fields = readable ? [{ kind: 'title_original', text: 'Synthetic title' }, { kind: 'body', text: 'Synthetic body' }] : []
  const get = vi.fn(async (path: string) => {
    if (missing) return { error: { error: { code: 'not_found' } }, response: { status: 404 } }
    let data: unknown
    if (path === '/api/v1/frameworks') data = { versions, totals: counts, access_unavailable: unavailable }
    else if (path.includes('/versions/')) data = { release: '20990101', items: [{ complete: true, units: [{ kind: 'head', version: framework }] }] }
    else if (path.includes('/controls/')) data = { release: '20990101', items: [{ complete: true, units: [{ kind: 'head', head: control }, ...fields] }] }
    else data = { release: '20990101', gaps: [] }
    return { data, response: { status: 200 } }
  })
  const post = vi.fn(async () => {
    const next = replies.shift()
    if (next instanceof Error) throw next
    return next || ({ response: { status: 200 }, data: { release: '20990101', abstain: false, frameworks: { [alpha]: framework, ...(mode === 'reduced' ? {} : { [beta]: betaFramework }) }, hits: [{ ...control, mappings: 0, lookup: true, ...(readable ? { snippet: 'Synthetic alpha snippet' } : {}) }, ...(mode === 'reduced' ? [] : [{ handle: `${beta}#B`, citation: 'B', kind: 'control', mappings: 0, text_withheld: false, lookup: true, snippet: 'Synthetic beta snippet' }])] } })
  })
  const clear = vi.fn()
  vi.stubGlobal('onBeforeUnmount', onBeforeUnmount)
  const notFound = vi.fn()
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useRoute', () => ({ params: { code: 'fw-alpha', version: '2.1' }, query: { citation: 'A' } }))
  vi.stubGlobal('useFrameworkPage', () => ({ language: ref('en'), notFound, api: { client: { GET: get, POST: post }, session: { csrfToken: 'synthetic' }, captureAction: () => ({ accountId: 1, csrfToken: 'synthetic' }), forAction: () => ({ POST: post }) } }))
  vi.stubGlobal('useAsyncData', async (_key: string, callback: () => Promise<unknown>) => ({ data: ref(await callback()), clear }))
  const root = defineComponent({ render: () => h(Suspense, null, { default: () => h(page) }) })
  const wrapper = mount(root, { global: { components: { FrameworkRead, FrameworkGaps, FrameworkUnits }, stubs: { NuxtLink: { template: '<a><slot /></a>' }, CorpusControl: true } } })
  return { wrapper, get, post, clear, notFound }
}
afterEach(() => vi.unstubAllGlobals())

for (const mode of ['reduced', 'mixed', 'full']) {
  it(`shows ${mode} data on the list, version, control and search pages`, async () => {
    for (const page of [List, Version, Control, Search]) {
      const { wrapper } = setup(mode, page)
      await flushPromises()
      if (page === List) expect(wrapper.text()).toContain('fw-alpha 2.1')
      if (page === Version) expect(wrapper.text()).toContain('Synthetic alpha')
      if (page === Control) {
        expect(wrapper.text()).toContain(mode === 'full' ? 'Synthetic body' : "The publisher's text needs access to read.")
        if (mode !== 'full') expect(wrapper.html()).not.toContain('Synthetic body')
      }
      if (page === Search) {
        await wrapper.get('input').setValue('Synthetic query')
        await wrapper.get('form').trigger('submit')
        await flushPromises()
        expect(wrapper.text()).toContain('Citation match')
        expect(wrapper.text().includes('Synthetic alpha snippet')).toBe(mode === 'full')
        expect(wrapper.text().includes('Synthetic beta snippet')).toBe(mode !== 'reduced')
      }
      wrapper.unmount()
    }
  })
}
it('shows the access unavailable notice from status', async () => {
  const { wrapper } = setup('mixed', List, true)
  await flushPromises()
  expect(wrapper.text()).toContain('Access could not be checked right now; this shows the free part.')
})

it('clears status data when leaving the list and search pages', async () => {
  for (const page of [List, Search]) {
    const { wrapper, clear } = setup('mixed', page)
    await flushPromises()
    wrapper.unmount()
    expect(clear).toHaveBeenCalledOnce()
  }
})

it('uses the fixed 404 for API 404 responses on each page', async () => {
  for (const page of [List, Search, Version, Control]) {
    const { wrapper, notFound } = setup('mixed', page, false, true)
    await flushPromises()
    expect(notFound).toHaveBeenCalled()
    wrapper.unmount()
  }
})

it('continues a search and restarts its submitted query when the cursor expires', async () => {
  const hit = { handle: `${alpha}#A`, citation: 'A', kind: 'control', mappings: 0, text_withheld: true, lookup: true }
  const makeResult = (citation: string, cursor?: string) => ({ response: { status: 200 }, data: { release: '20990101', abstain: false, hits: [{ ...hit, citation }], next_cursor: cursor } })
  const replies = [makeResult('first', 'cursor-a'), { response: { status: 409 }, error: { error: { code: 'cursor_expired' } } }, makeResult('fresh')]
  const { wrapper, post } = setup('mixed', Search, false, false, replies)
  await flushPromises()
  await wrapper.get('input').setValue('original query')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  await wrapper.get('input').setValue('different query')
  await wrapper.get('button[type="button"]').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('The results changed. Please search again.')
  expect(wrapper.text()).toContain('fresh')
  expect(wrapper.text()).not.toContain('first')
  expect(post).toHaveBeenCalledTimes(3)
  expect(post.mock.calls[2]?.[1]).toMatchObject({ body: { query: 'original query' } })
})

it('appends a search page using the original cursor, release and filter', async () => {
  const hit = (citation: string) => ({ handle: `${alpha}#${citation}`, citation, kind: 'control', mappings: 0, text_withheld: true, lookup: true })
  const replies = [
    { response: { status: 200 }, data: { release: '20990101', abstain: false, hits: [hit('first')], next_cursor: 'cursor-a' } },
    { response: { status: 200 }, data: { release: '20990101', abstain: false, hits: [hit('second')] } },
  ]
  const { wrapper, post } = setup('mixed', Search, false, false, replies)
  await flushPromises()
  await wrapper.get('input').setValue('synthetic query')
  await wrapper.get('select').setValue('fw-alpha')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  await wrapper.get('button[type="button"]').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('first')
  expect(wrapper.text()).toContain('second')
  expect(post.mock.calls[1]?.[1]).toMatchObject({ body: { cursor: 'cursor-a', release: '20990101', frameworks: ['fw-alpha'], query: 'synthetic query' } })
})

it('shows a search error when the request throws', async () => {
  const { wrapper } = setup('mixed', Search, false, false, [new Error('offline')])
  await flushPromises()
  await wrapper.get('input').setValue('synthetic query')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toBe('Please try again later.')
  expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
})
