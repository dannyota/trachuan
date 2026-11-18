import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createInterface } from 'node:readline'

export async function withPublicSite(available: boolean, run: (origin: string) => Promise<void>, children: object[] = []) {
  const node = { code: '1', name: 'Chủ đề mẫu', documents: 2, provisions: 4, children }
  const api = createServer((request, response) => {
    if (!available) {
      response.writeHead(503).end()
      return
    }
    const path = new URL(request.url!, 'http://localhost').pathname
    const data = path === '/api/v1/public/status' ? { release: '20990101_1', documents: 2, provisions: 4, nodes: 1 } : path === '/api/v1/public/taxonomy/codes' ? { release: '20990101_1', codes: ['1'] } : path === '/api/v1/public/taxonomy' ? { release: '20990101_1', nodes: [node], truncated: false } : path === '/api/v1/public/taxonomy/1' ? { release: '20990101_1', node, path: [node], documents: [] } : null
    response.writeHead(data ? 200 : 404, { 'content-type': 'application/json' })
    response.end(JSON.stringify(data))
  })
  api.listen(0, '127.0.0.1')
  await once(api, 'listening')
  const apiPort = (api.address() as { port: number }).port
  const server = spawn(process.execPath, ['.output/server/index.mjs'], {
    env: { ...process.env, NITRO_HOST: '127.0.0.1', NITRO_PORT: '0', NUXT_API_ORIGIN: `http://127.0.0.1:${apiPort}` },
    stdio: ['ignore', 'pipe', 'ignore'],
  })
  try {
    const origin = await new Promise<string>((resolve, reject) => {
      server.once('exit', code => reject(new Error(`site exited with code ${code}`)))
      createInterface({ input: server.stdout }).on('line', line => {
        const match = /Listening on (http:\/\/127\.0\.0\.1:\d+)/.exec(line)
        if (match) resolve(match[1]!)
      })
    })
    await run(origin)
  } finally {
    server.kill()
    await new Promise<void>(resolve => api.close(() => resolve()))
  }
}
