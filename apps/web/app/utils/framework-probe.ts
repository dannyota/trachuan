type Storage = { getItem: (key: string) => string | null, setItem: (key: string, value: string) => void }
const pending = new WeakMap<Storage, Promise<boolean>>()
export async function probeFrameworks(storage: Storage, probe: () => Promise<boolean>): Promise<boolean> {
  const key = 'frameworks-probe'
  try {
    const saved = storage.getItem(key)
    if (saved !== null) return saved === 'yes'
    const current = pending.get(storage)
    if (current) return await current
    const request = probe().catch(() => false)
    pending.set(storage, request)
    try {
      const available = await request
      storage.setItem(key, available ? 'yes' : 'no')
      return available
    } finally { pending.delete(storage) }
  } catch { return false }
}
export function tabFrameworkProbe(probe: () => Promise<boolean>): Promise<boolean> {
  try { return probeFrameworks(window.sessionStorage, probe) } catch { return Promise.resolve(false) }
}
