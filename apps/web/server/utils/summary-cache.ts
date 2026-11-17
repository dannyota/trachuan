import { Buffer } from 'node:buffer'
import type { Driver } from 'unstorage'

export function vietnamDate(now: number): string {
  return new Date(now + 7 * 3600_000).toISOString().slice(0, 10)
}

export function summaryCacheDriver(clock: () => number): Driver {
  const entries = new Map<string, { value: string, bytes: number, expires: number }>()
  let bytes = 0
  function remove(key: string) {
    bytes -= entries.get(key)?.bytes ?? 0
    entries.delete(key)
  }
  function purge() {
    for (const [key, entry] of entries) if (clock() >= entry.expires) remove(key)
  }
  return {
    name: 'summary-memory',
    hasItem(key) { purge(); return entries.has(key) },
    getItem(key) { purge(); return entries.get(key)?.value ?? null },
    setItem(key, value) {
      purge()
      remove(key)
      const entry = JSON.parse(value) as { value?: { body?: unknown } }
      if (typeof entry.value?.body !== 'string' || Buffer.byteLength(entry.value.body) > 256 * 1024) return
      const size = Buffer.byteLength(value)
      if (size > 32 * 1024 * 1024) return
      while (bytes + size > 32 * 1024 * 1024) remove(entries.keys().next().value!)
      const now = clock()
      const midnight = Date.parse(`${vietnamDate(now)}T00:00:00+07:00`) + 86400_000
      entries.set(key, { value, bytes: size, expires: Math.min(now + 300_000, midnight) })
      bytes += size
    },
    removeItem: remove,
    getKeys() { purge(); return [...entries.keys()] },
    clear() { entries.clear(); bytes = 0 },
  }
}
