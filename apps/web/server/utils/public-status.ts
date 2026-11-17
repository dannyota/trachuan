import createClient from 'openapi-fetch'
import type { components, paths } from '@trachuan/schema/gen/openapi'

type PublicStatus = components['schemas']['PublicStatus']
type Entry = { started: number, value?: PublicStatus, pending?: Promise<PublicStatus | undefined> }

const cacheKey = Symbol.for('trachuan.public-status')
const state = globalThis as typeof globalThis & { [cacheKey]?: Map<string, Entry> }
const cache = state[cacheKey] ??= new Map<string, Entry>()

export function publicStatus(origin: string, after?: number): Promise<PublicStatus | undefined> {
  const current = cache.get(origin)
  if (current?.pending && (after === undefined || current.started > after)) return current.pending
  if (current?.value && (after === undefined || current.started > after) && Date.now() - current.started < 30_000) return Promise.resolve(current.value)

  const entry: Entry = { started: Date.now() }
  entry.pending = Promise.resolve().then(async () => {
    try {
      const client = createClient<paths>({ baseUrl: origin, credentials: 'omit' })
      const { data, response } = await client.GET('/api/v1/public/status', { signal: AbortSignal.timeout(3000) })
      if (response.ok && data && typeof data === 'object' && !Array.isArray(data) && typeof data.release === 'string') {
        entry.value = data
        return data
      }
    } catch {
      return undefined
    } finally {
      entry.pending = undefined
      if (!entry.value && cache.get(origin) === entry) cache.delete(origin)
    }
    return undefined
  })
  cache.set(origin, entry)
  return entry.pending
}
