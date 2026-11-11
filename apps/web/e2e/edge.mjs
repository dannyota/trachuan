import http from 'node:http'

const port = Number(process.env.E2E_EDGE_PORT || 28083)
const serverPort = Number(process.env.E2E_SERVER_PORT || 28080)
const webPort = Number(process.env.E2E_WEB_PORT || 28084)

http.createServer((req, res) => {
  const path = req.url.split('?', 1)[0]
  if (path === '/healthz' || path === '/readyz') {
    res.writeHead(404).end()
    return
  }
  const api = path.startsWith('/api/') || path.startsWith('/mcp') || path.startsWith('/oauth/') || path.startsWith('/.well-known/oauth-')
  const forwarded = [req.headers['x-forwarded-for'], req.socket.remoteAddress].filter(Boolean).join(', ')
  const headers = { ...req.headers, 'x-forwarded-for': forwarded }
  const proxy = http.request({ host: '127.0.0.1', port: api ? serverPort : webPort, method: req.method, path: req.url, headers }, upstream => {
    res.writeHead(upstream.statusCode, upstream.headers)
    upstream.pipe(res)
  })
  proxy.on('error', () => { if (!res.headersSent) res.writeHead(502); res.end() })
  req.pipe(proxy)
}).listen(port, '127.0.0.1', () => console.log(`edge listening on 127.0.0.1:${port}`))
