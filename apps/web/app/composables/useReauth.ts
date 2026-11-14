import { ref } from 'vue'
import { reauthDecision } from '../utils/account-security'
import { useSessionStore } from '../stores/session'
import { loginUrl } from '../api/next'
import type { Action } from '../api/client'

export function useReauth() {
  const session = useSessionStore()
  const challenge = ref<{ action: Action, factor: boolean } | null>(null)
  let retry: ((code?: string) => Promise<void>) | null = null
  function requireProof(error: { error?: { code?: string }, factor?: boolean }, action: Action, acceptsCode: boolean, operation: (code?: string) => Promise<void>): boolean {
    const decision = reauthDecision(session.account, error, acceptsCode)
    if (!decision) return false
    if (decision.kind === 'login') window.location.assign(loginUrl('/account'))
    else { challenge.value = { action, factor: decision.factor }; retry = operation }
    return true
  }
  function cancel() { challenge.value = null; retry = null }
  async function verified(code?: string) {
    const operation = retry
    cancel()
    await operation?.(code)
  }
  return { challenge, requireProof, cancel, verified }
}
