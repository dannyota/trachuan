// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import CorpusControl from '../app/components/CorpusControl.vue'

it('moves to the other corpus without carrying a query', async () => {
  const push = vi.fn()
  vi.stubGlobal('useState', () => ({ value: 'en' }))
  vi.stubGlobal('useRouter', () => ({ push }))
  const wrapper = mount(CorpusControl, { props: { current: 'law' } })
  await wrapper.get('select').setValue('frameworks')
  expect(push).toHaveBeenCalledWith('/frameworks/search')
  vi.unstubAllGlobals()
})
