import { expect, it } from 'vitest'
import { createStorage } from 'unstorage'
import { summaryCacheDriver, vietnamDate } from '../server/utils/summary-cache'

it('expires at 300 seconds or Vietnam midnight and counts stored bytes', async () => {
  let now = Date.parse('2099-01-01T16:59:59Z')
  const storage = createStorage({ driver: summaryCacheDriver(() => now) })
  const entry = { value: { body: 'summary' } }
  await storage.setItem('one', entry)
  expect(await storage.getItem('one')).toEqual(entry)
  expect(vietnamDate(now)).toBe('2099-01-01')
  now += 1000
  expect(vietnamDate(now)).toBe('2099-01-02')
  expect(await storage.getItem('one')).toBeNull()
  await storage.setItem('two', entry)
  now += 300_000
  expect(await storage.getItem('two')).toBeNull()
})

it('serves but never stores rendered HTML over 256 KiB', async () => {
  const storage = createStorage({ driver: summaryCacheDriver(() => 0) })
  await storage.setItem('limit', { value: { body: 'a'.repeat(256 * 1024) } })
  expect(await storage.hasItem('limit')).toBe(true)
  await storage.setItem('oversize', { value: { body: 'á'.repeat(128 * 1024 + 1) } })
  expect(await storage.getItem('oversize')).toBeNull()
})

it('evicts oldest entries to bound the whole cache at 32 MiB', async () => {
  const storage = createStorage({ driver: summaryCacheDriver(() => 0) })
  for (let i = 0; i < 140; i++) await storage.setItem(String(i), { value: { body: 'a'.repeat(256 * 1024) } })
  expect(await storage.getItem('0')).toBeNull()
  expect(await storage.getItem('139')).not.toBeNull()
  const keys = await storage.getKeys()
  let size = 0
  for (const key of keys) size += Buffer.byteLength(JSON.stringify(await storage.getItem(key)))
  expect(size).toBeLessThanOrEqual(32 * 1024 * 1024)
})
