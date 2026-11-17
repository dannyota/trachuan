import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createInterface } from 'node:readline'
import { forbiddenMarkers, publicSummary } from './summary-data'

export type SummaryState = { available: boolean, release: string | null, calls: { path: string, cookie: string }[], pageCookies: string[] }

export async function withSummarySite(run: (origin: string, state: SummaryState) => Promise<void>) {
  const state: SummaryState = { available: true, release: publicSummary.release, calls: [], pageCookies: [] }
  let webOrigin = ''
  const api = createServer(async (request, response) => {
    const url = new URL(request.url!, 'http://localhost')
    const path = decodeURIComponent(url.pathname)
    if (request.method === 'GET' && path === '/__fixture/session') {
      const value = url.searchParams.get('value') ?? ''
      if (!['valid', 'expired', 'forged'].includes(value)) { response.writeHead(400).end(); return }
      response.writeHead(200, {
        'set-cookie': `__Host-session=${value}; Path=/; Secure; HttpOnly; SameSite=Lax`,
        'cache-control': 'no-store',
        'content-type': 'text/html; charset=utf-8',
      }).end('<!doctype html><html><head><title>Session fixture</title></head><body>Session fixture</body></html>')
      return
    }
    if (!path.startsWith('/api/')) {
      if (path.endsWith('/summary')) state.pageCookies.push(request.headers.cookie ?? '')
      try {
        const upstream = await fetch(webOrigin + request.url, { headers: request.headers as Record<string, string>, redirect: 'manual' })
        const headers = new Headers(upstream.headers)
        headers.delete('content-encoding')
        headers.delete('content-length')
        response.writeHead(upstream.status, Object.fromEntries(headers))
        response.end(Buffer.from(await upstream.arrayBuffer()))
      } catch { response.writeHead(503).end() }
      return
    }
    state.calls.push({ path, cookie: request.headers.cookie ?? '' })
    const headers = { 'content-type': 'application/json' }
    if (path === '/api/v1/me') {
      if (request.headers.cookie?.split(/;\s*/).includes('__Host-session=valid')) response.writeHead(200, headers).end(JSON.stringify({ id: 1, email: 'summary@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'synthetic-csrf' }))
      else response.writeHead(204).end()
      return
    }
    if (!path.startsWith('/api/v1/public/')) {
      response.writeHead(200, headers).end(JSON.stringify({ markers: forbiddenMarkers }))
      return
    }
    if (!state.release || path.endsWith('/vn:vbpl:8')) { response.writeHead(503, headers).end('{}'); return }
    const node = { code: '1', name: 'Chủ đề mẫu', documents: 1, provisions: 1 }
    const status = { release: state.release, documents: 1, provisions: 1, nodes: 1, features: { public_summaries: state.available } }
    if (path === '/api/v1/public/status') { response.writeHead(200, headers).end(JSON.stringify(status)); return }
    if (path === '/api/v1/public/taxonomy/1') { response.writeHead(200, headers).end(JSON.stringify({ release: state.release, node, path: [node], documents: [{ key: 'vn:vbpl:1', number: '1/2099/QH', title: 'Luật mẫu' }] })); return }
    if (path === '/api/v1/public/taxonomy/codes') { response.writeHead(200, headers).end(JSON.stringify({ release: state.release, codes: ['1'] })); return }
    if (!state.available || path.endsWith('/vn:vbpl:9')) { response.writeHead(404, headers).end('{}'); return }
    if (path === '/api/v1/public/documents') { response.writeHead(200, headers).end(JSON.stringify({ release: state.release, projection: 1, total: 1, items: [{ key: 'vn:vbpl:1', index: true }] })); return }
    if (path.startsWith('/api/v1/public/documents/')) {
      const key = path.slice('/api/v1/public/documents/'.length)
      const summary = { ...publicSummary, release: state.release, document: { ...publicSummary.document, key }, relations: [...publicSummary.relations, { kind: 'cites', direction: 'outgoing', key: 'vn:vbpl:9', number: 'MISSING' }, { kind: 'cites', direction: 'outgoing', key: 'vn:vbpl:8', number: 'UNAVAILABLE' }], relations_total: 4 }
      response.writeHead(200, headers).end(JSON.stringify(summary))
      return
    }
    response.writeHead(404, headers).end('{}')
  })
  api.listen(0, '127.0.0.1')
  await once(api, 'listening')
  const origin = `http://127.0.0.1:${(api.address() as { port: number }).port}`
  const server = spawn(process.execPath, ['.output/server/index.mjs'], {
    env: { ...process.env, NITRO_HOST: '127.0.0.1', NITRO_PORT: '0', NUXT_API_ORIGIN: origin, NUXT_TRUSTED_PROXIES: '127.0.0.1/32' },
    stdio: ['ignore', 'pipe', 'ignore'],
  })
  try {
    webOrigin = await new Promise<string>((resolve, reject) => {
      server.once('exit', code => reject(new Error(`site exited with code ${code}`)))
      createInterface({ input: server.stdout }).on('line', line => {
        const match = /Listening on (http:\/\/127\.0\.0\.1:\d+)/.exec(line)
        if (match) resolve(match[1]!)
      })
    })
    await run(origin, state)
  } finally {
    server.kill()
    await new Promise<void>(resolve => api.close(() => resolve()))
  }
}
