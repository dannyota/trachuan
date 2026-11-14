import createClient from 'openapi-fetch'
import type { paths } from '@trachuan/schema/gen/openapi'

export function useAuthApi() {
  return createClient<paths>({ baseUrl: import.meta.client ? window.location.origin : 'http://localhost', fetch: input => fetch(new Request(input as Request, { cache: 'no-store' })) })
}
