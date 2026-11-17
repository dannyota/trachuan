import createClient from 'openapi-fetch'
import type { paths } from '@trachuan/schema/gen/openapi'

export function createSummaryApi(origin: string, transport: typeof globalThis.fetch = globalThis.fetch) {
  const client = createClient<paths>({
    baseUrl: origin,
    credentials: 'omit',
    fetch: (input: Request) => {
      const headers = new Headers(input.headers)
      headers.delete('cookie')
      headers.delete('authorization')
      return transport(new Request(input, { headers, credentials: 'omit' }))
    },
  })
  return {
    status: (signal?: AbortSignal) => client.GET('/api/v1/public/status', { signal }),
    summary: (key: string, signal?: AbortSignal) => client.GET('/api/v1/public/documents/{key}', { params: { path: { key } }, signal }),
  }
}
