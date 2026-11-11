import { expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import * as apiClient from '../app/api/client'
import { useApi } from '../app/composables/useApi'

it('enables 401 navigation for signed-in route metadata and user actions', () => {
  const options: Array<Parameters<typeof apiClient.createApi>[0]> = []
  const spy = vi.spyOn(apiClient, 'createApi').mockImplementation((value) => {
    options.push(value)
    return {} as ReturnType<typeof apiClient.createApi>
  })
  const unavailable = { value: false }
  vi.stubGlobal('useState', () => unavailable)
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } })
  setActivePinia(createPinia())
  try {
    useApi({ route: { path: '/future', fullPath: '/future', meta: { signedIn: true } } })
    useApi({ route: { path: '/account', fullPath: '/account', meta: {} } })
    useApi({ userAction: true, route: { path: '/public', fullPath: '/public', meta: {} } })
    expect(options.map(option => typeof option.onUnauthorized)).toEqual(['function', 'undefined', 'function'])
    options[0]?.onUnavailable?.()
    expect(unavailable.value).toBe(true)
  } finally {
    spy.mockRestore()
    vi.unstubAllGlobals()
  }
})
