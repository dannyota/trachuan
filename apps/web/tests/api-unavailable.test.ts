import { expect, it } from 'vitest'
import { createApi } from '../app/api/client'

it('normalizes transport failures and invalid or server responses', async () => {
  for (const transport of [
    async () => { throw new Error('private transport detail') },
    async () => new Response('bad gateway', { status: 503 }),
    async () => new Response('not json', { status: 200 }),
    async () => new Response('{', { status: 200, headers: { 'content-type': 'application/json' } }),
  ]) {
    const api = createApi({ origin: 'http://localhost', fetch: transport })
    await expect(api.loadSession()).rejects.toMatchObject({ code: 'unavailable', status: 503, message: 'unavailable' })
    const response = await api.client.GET('/api/v1/status')
    expect(response.response.status).toBe(503)
    expect(response.error).toEqual({ error: { code: 'unavailable', message: 'unavailable' } })
  }
})
