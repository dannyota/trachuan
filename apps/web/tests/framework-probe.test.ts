import { expect, it, vi } from 'vitest'
import { probeFrameworks } from '../app/utils/framework-probe'

it('probes once per tab and stores only availability', async () => {
  const values = new Map<string, string>()
  const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) } }
  const probe = vi.fn(async () => true)
  expect(await probeFrameworks(storage, probe)).toBe(true)
  expect(await probeFrameworks(storage, probe)).toBe(true)
  expect(probe).toHaveBeenCalledTimes(1)
  expect([...values.values()]).toEqual(['yes'])
})
it('silently records failure', async () => {
  const storage = { getItem: () => null, setItem: vi.fn() }
  expect(await probeFrameworks(storage, async () => { throw new Error('offline') })).toBe(false)
  expect(storage.setItem).toHaveBeenCalledWith('frameworks-probe', 'no')
})

it('shares an in-flight probe between visits in the same tab', async () => {
  const values = new Map<string, string>()
  const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) } }
  let finish!: (available: boolean) => void
  const probe = vi.fn(() => new Promise<boolean>(resolve => { finish = resolve }))
  const first = probeFrameworks(storage, probe)
  const second = probeFrameworks(storage, probe)
  expect(probe).toHaveBeenCalledOnce()
  finish(true)
  expect(await Promise.all([first, second])).toEqual([true, true])
})
