import { computed, ref } from 'vue'
import { vi } from 'vitest'

export function setupReader(pages: unknown[], query: Record<string, string> = {}) {
  const calls: unknown[] = []
  vi.stubGlobal('IntersectionObserver', undefined)
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query, hash: '' }))
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('useRouter', () => ({ currentRoute: { get value() {
    const route = (globalThis as unknown as { useRoute: () => { params: { key: string }, query: Record<string, unknown>, hash: string } }).useRoute()
    return { path: `/vn/documents/${route.params.key}`, query: route.query, hash: route.hash }
  } } }))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('useApi', () => ({ client: { GET: async (_path: string, options: unknown) => {
    calls.push(options)
    return pages.shift()
  } } }))
  return calls
}
