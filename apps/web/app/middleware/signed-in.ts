import { loginUrl } from '../api/next'

export default defineNuxtRouteMiddleware(async (to) => {
  if (to.path !== '/account' && to.path !== '/vn' && !to.path.startsWith('/vn/')) return
  const api = useApi({ route: to })
  if (api.session.status === 'unknown') await api.loadSession()
  if (api.session.status === 'signed-out') return navigateTo(loginUrl(to.fullPath), { external: true })
})
