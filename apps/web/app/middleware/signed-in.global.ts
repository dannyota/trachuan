import { loginUrl } from '../api/next'
import { frameworkNext } from '../utils/framework-handles'
import { ApiError } from '../api/client'

export default defineNuxtRouteMiddleware(async (to) => {
  const unavailable = useState<boolean>('service-unavailable', () => false)
  unavailable.value = false
  if (to.meta.signedIn !== true) return
  const api = useApi({ route: to })
  if (api.session.status === 'unknown') {
    try {
      await api.loadSession()
    } catch (error) {
      if (!(error instanceof ApiError) || error.code !== 'unavailable') throw error
      return
    }
  }
  if (api.session.status === 'signed-out') return navigateTo(loginUrl(to.path.startsWith('/frameworks') ? frameworkNext(to.fullPath) : to.fullPath), { external: true })
})
