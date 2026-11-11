import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { spawn } from 'node:child_process'
import { once } from 'node:events'

async function listen(server) {
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  return server.address().port
}

test('edge routes requests and preserves the forwarded request', async (t) => {
  const upstream = name => createServer((req, res) => {
    let body = ''
    req.on('data', chunk => { body += chunk })
    req.on('end', () => res.end(JSON.stringify({ name, url: req.url, method: req.method, body, forwarded: req.headers['x-forwarded-for'] })))
  })
  const go = upstream('go'), web = upstream('web')
  const goPort = await listen(go), webPort = await listen(web)
  const reservation = createServer()
  const edgePort = await listen(reservation)
  await new Promise(resolve => reservation.close(resolve))
  const child = spawn(process.execPath, ['e2e/edge.mjs'], { cwd: new URL('..', import.meta.url), env: { ...process.env, E2E_SERVER_PORT: String(goPort), E2E_WEB_PORT: String(webPort), E2E_EDGE_PORT: String(edgePort) } })
  t.after(() => { child.kill(); go.close(); web.close() })
  await Promise.race([once(child.stdout, 'data'), once(child, 'exit').then(() => { throw new Error('edge exited') })])
  const origin = `http://127.0.0.1:${edgePort}`
  for (const path of ['/api/test?q=1', '/mcp', '/oauth/test', '/.well-known/oauth-test']) {
    const response = await fetch(origin + path, { method: 'POST', headers: { 'x-forwarded-for': '192.0.2.1' }, body: 'body' })
    const result = await response.json()
    assert.deepEqual(result, { name: 'go', url: path, method: 'POST', body: 'body', forwarded: '192.0.2.1, 127.0.0.1' })
  }
  const direct = await (await fetch(origin + '/account')).json()
  assert.equal(direct.name, 'web')
  assert.equal(direct.forwarded, '127.0.0.1')
  for (const path of ['/healthz', '/readyz']) assert.equal((await fetch(origin + path)).status, 404)
})
