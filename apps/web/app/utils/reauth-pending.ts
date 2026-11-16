export type ReauthAction = 'token' | 'password-link' | 'authenticator'
const key = 'reauth-action'
type PendingStorage = Pick<Storage, 'getItem' | 'removeItem' | 'setItem'>

export function savePendingReauth(storage: PendingStorage, action: ReauthAction, now = Date.now()) {
  try { storage.setItem(key, JSON.stringify({ action, at: now })) } catch { return }
}

export function takePendingReauth(storage: PendingStorage, now = Date.now()): ReauthAction | null {
  try {
    const raw = storage.getItem(key)
    storage.removeItem(key)
    if (!raw) return null
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== 'object' || !('action' in value) || !('at' in value)) return null
    if (value.action !== 'token' && value.action !== 'password-link' && value.action !== 'authenticator') return null
    if (typeof value.at !== 'number' || !Number.isFinite(value.at) || value.at > now || now - value.at >= 600000) return null
    return value.action
  } catch { return null }
}
