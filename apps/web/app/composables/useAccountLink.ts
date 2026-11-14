import { computed, onMounted, ref } from 'vue'
import type { components } from '@trachuan/schema/gen/openapi'
import { authError, authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import type { Action } from '../api/client'
import { loginUrl } from '../api/next'
import { usePageSecrets } from './usePageSecrets'

export function useAccountLink(token: string) {
  const language = useState<Language>('language', () => 'vi')
  const labels = computed(() => authMessages(language.value))
  const client = useAuthApi()
  const api = useApi({ localUnavailable: true })
  const link = ref<components['schemas']['EmailLink'] | null>(null)
  const password = ref('')
  const failure = ref<{ code?: string, status?: number } | null>(null)
  const signInRequired = ref(false)
  const error = computed(() => signInRequired.value ? labels.value.linkSignIn : failure.value ? authError(language.value, failure.value.code, failure.value.status) : '')
  const signInHref = computed(() => signInRequired.value ? loginUrl('/account') : '/login')
  const busy = ref(false)
  const saved = ref(false)
  let action: Action | null = null
  function clear() { token = ''; password.value = ''; link.value = null; action = null }
  function requireSignIn() { clear(); signInRequired.value = true; failure.value = null }
  function fail(code?: string, status?: number) {
    failure.value = { code, status }
    if (code === 'link_invalid') clear()
  }
  const secrets = usePageSecrets(clear, () => { if (!signInRequired.value && !saved.value) fail() })
  onMounted(async () => {
    const signal = secrets.signal
    if (!token) { fail(); return }
    try {
      const result = await client.POST('/api/v1/auth/link', { params: { header: { Origin: window.location.origin } }, body: { token }, signal })
      if (signal.aborted) return
      if (!result.data) { fail(result.error?.error.code, result.response.status); return }
      if (result.data.purpose === 'add_password') {
        await api.loadSession(signal)
        if (signal.aborted) return
        if (!api.session.account) { requireSignIn(); return }
        action = api.captureAction()
      }
      link.value = result.data
    } catch { if (!signal.aborted) fail() }
  })
  async function save() {
    const signal = secrets.signal
    if (busy.value || !link.value || signal.aborted) return
    busy.value = true; failure.value = null
    const purpose = link.value.purpose
    try {
      if (purpose === 'add_password') {
        await api.loadSession(signal)
        if (signal.aborted) return
        if (!action || !api.session.account || action.accountId !== api.session.account.id) { requireSignIn(); return }
      }
      const body = { token, password: password.value }
      const result = purpose === 'add_password'
        ? await api.forAction(action!).POST('/api/v1/me/password', { params: { header: { 'X-CSRF-Token': '' } }, body, signal })
        : await client.POST(purpose === 'reset' ? '/api/v1/auth/reset/complete' : '/api/v1/auth/signup/complete', { params: { header: { Origin: window.location.origin } }, body, signal })
      if (signal.aborted) return
      if (result.error) {
        const code = result.error.error.code as string
        if (purpose === 'add_password' && (code === 'account_changed' || code === 'unauthenticated' || (result.response.status === 403 && code === 'reauth_required'))) requireSignIn()
        else fail(code, result.response.status)
        return
      }
      clear()
      if (purpose === 'reset') { saved.value = true; api.session.account = null; api.session.csrfToken = null; api.session.status = 'signed-out' }
      else window.location.assign('/account')
    } catch { if (!signal.aborted) fail() }
    finally { busy.value = false }
  }
  return { labels, link, password, error, busy, saved, signInHref, save }
}
