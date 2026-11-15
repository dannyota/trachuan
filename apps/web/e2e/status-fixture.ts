import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createInterface } from 'node:readline'

export async function withStatusSite(status: Record<string, unknown>, run: (origin: string) => Promise<void>) {
  const api = createServer((_request, response) => {
    response.writeHead(200, { 'content-type': 'application/json' })
    response.end(JSON.stringify(status))
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
