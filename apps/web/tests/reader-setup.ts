import { computed, ref } from 'vue'
import { vi } from 'vitest'

export function setupReader(pages: unknown[], query: Record<string, string> = {}) {
  const calls: unknown[] = []
  vi.stubGlobal('definePageMeta', () => {})
  vi.stubGlobal('useRoute', () => ({ params: { key: 'vn:vbpl:1' }, query, hash: '' }))
  vi.stubGlobal('useState', () => ref('en'))
  vi.stubGlobal('computed', computed)
  vi.stubGlobal('ref', ref)
  vi.stubGlobal('useApi', () => ({ client: { GET: async (_path: string, options: unknown) => {
    calls.push(options)
    return pages.shift()
  } } }))
  return calls
}
