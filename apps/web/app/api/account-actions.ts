import type { createApi } from './client'

type Client = ReturnType<typeof createApi>['client']
const header = { 'X-CSRF-Token': '' }

export function accountActions(client: Client) {
  return {
    createToken: (name: string, days: number) => client.POST('/api/v1/me/tokens', { params: { header }, body: { name, days } }),
    revokeToken: (id: number) => client.DELETE('/api/v1/me/tokens/{id}', { params: { header, path: { id } } }),
    revokeGrant: (id: number) => client.DELETE('/api/v1/me/grants/{id}', { params: { header, path: { id } } }),
    revokeSession: (session: { id: number, current: boolean }) => session.current
      ? client.POST('/api/v1/auth/logout', { params: { header } })
      : client.DELETE('/api/v1/me/sessions/{id}', { params: { header, path: { id: session.id } } }),
    deleteAccount: () => client.DELETE('/api/v1/me', { params: { header } }),
  }
}
