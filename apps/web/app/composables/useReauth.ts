import { ref } from 'vue'
import { reauthDecision } from '../utils/account-security'
import { useSessionStore } from '../stores/session'
import { savePendingReauth, type ReauthAction } from '../utils/reauth-pending'
import type { Action } from '../api/client'

export function useReauth() {
  const session = useSessionStore()
  const challenge = ref<{ action: Action, factor: boolean } | null>(null)
  const notice = ref<ReauthAction | null>(null)
  let retry: ((code?: string) => Promise<void>) | null = null
  function requireProof(error: { error?: { code?: string }, factor?: boolean }, action: Action, pending: ReauthAction, acceptsCode: boolean, operation: (code?: string) => Promise<void>): boolean {
    const decision = reauthDecision(session.account, error, acceptsCode)
    if (!decision) return false
    if (decision.kind === 'login') notice.value = pending
    else { challenge.value = { action, factor: decision.factor }; retry = operation }
    return true
  }
  function cancel() { challenge.value = null; notice.value = null; retry = null }
  function signInAgain() {
    if (!notice.value) return
    try { savePendingReauth(window.sessionStorage, notice.value) } catch { notice.value = null }
    window.location.assign('/login?next=%2Faccount&reason=reauth')
  }
  async function verified(code?: string) {
    const operation = retry
    cancel()
    await operation?.(code)
  }
  return { challenge, notice, requireProof, cancel, verified, signInAgain }
}
