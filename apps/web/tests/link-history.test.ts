// @vitest-environment happy-dom
import { expect, it } from 'vitest'
import { createRouter, createWebHistory } from 'vue-router'
import { takeLinkToken } from '../app/utils/account-security'

it('never restores the token when Vue Router navigates away', async () => {
  window.history.replaceState(null, '', '/link#synthetic-token')
  const history = createWebHistory()
  const router = createRouter({ history, routes: [{ path: '/link', component: {} }, { path: '/login', component: {} }] })
  await router.replace('/link#synthetic-token')
  const token = takeLinkToken(window.location, window.history, history)
  expect(token).toBe('synthetic-token')
  expect(window.location.hash).toBe('')
  expect(JSON.stringify(window.history.state)).not.toContain('synthetic-token')
  await router.push('/login')
  expect(JSON.stringify(window.history.state)).not.toContain('synthetic-token')
  expect(window.history.state.back).toBe('/link')
  history.destroy()
})
