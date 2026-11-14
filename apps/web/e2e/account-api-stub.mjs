import { createServer } from 'node:http'

createServer((request, response) => {
  if (request.url === '/ready') {
    response.writeHead(204).end()
    return
  }
  if (request.method === 'GET' && request.url === '/api/v1/me' && request.headers.cookie?.split(/;\s*/).includes('account-fixture=1')) {
    response.writeHead(200, { 'content-type': 'application/json' })
    const security = request.headers.cookie?.match(/(?:^|;\s*)account-security=([01]{3})(?:;|$)/)?.[1] || '101'
    response.end(JSON.stringify({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf', password: security[0] === '1', google: security[0] === '0', totp: security[1] === '1', totp_enrollment: security[2] === '1' }))
    return
  }
  response.writeHead(503, { 'content-type': 'application/json' })
  response.end(JSON.stringify({ error: { code: 'unavailable', message: 'unavailable' } }))
}).listen(3032, '127.0.0.1')
