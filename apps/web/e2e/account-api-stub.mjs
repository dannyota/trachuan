import { createServer } from 'node:http'

createServer((request, response) => {
  if (request.url === '/ready') {
    response.writeHead(204).end()
    return
  }
  if (request.method === 'GET' && request.url === '/api/v1/me' && request.headers.cookie?.split(/;\s*/).includes('account-fixture=1')) {
    response.writeHead(200, { 'content-type': 'application/json' })
    response.end(JSON.stringify({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'csrf' }))
    return
  }
  response.writeHead(503, { 'content-type': 'application/json' })
  response.end(JSON.stringify({ error: { code: 'unavailable', message: 'unavailable' } }))
}).listen(3032, '127.0.0.1')
