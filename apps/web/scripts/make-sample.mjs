import { readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'

const repoRoot = new URL('../../../', import.meta.url)
const defaultOutput = fileURLToPath(new URL('../app/data/', import.meta.url))
const output = resolve(process.env.TRACHUAN_SAMPLE_DIR || defaultOutput)
const cases = JSON.parse(await readFile(new URL('apps/corpus/internal/evaluation/testdata/vn/legacy.json', repoRoot), 'utf8'))
const chosen = cases.find(item => item.id === 'atttt-50-2024')
const base = process.env.TRACHUAN_URL
const token = process.env.TRACHUAN_TOKEN
if (!base || !token || !chosen) throw new Error('Sample input is unavailable')
const url = new URL('/mcp', base)
let session
async function send(body, notification = false) {
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream', 'MCP-Protocol-Version': '2025-06-18' }
  if (session) headers['Mcp-Session-Id'] = session
  const response = await fetch(url, { method: 'POST', headers, body: JSON.stringify(body) })
  if (!response.ok) throw new Error('MCP request failed')
  session = response.headers.get('mcp-session-id') || session
  if (notification) return
  const text = await response.text()
  let payload
  if (response.headers.get('content-type')?.includes('text/event-stream')) {
    const events = text.split(/\r?\n\r?\n/)
    const messages = events.map(event => event.split(/\r?\n/).filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')).filter(Boolean)
    payload = messages.map(message => JSON.parse(message)).find(message => message.id === body.id)
  } else payload = JSON.parse(text)
  if (!payload || payload.error || payload.result?.isError || !payload.result) throw new Error('MCP request failed')
  return payload.result
}
await send({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'trachuan-sample', version: '1.0.0' } } })
await send({ jsonrpc: '2.0', method: 'notifications/initialized' }, true)
const result = await send({ jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: 'search', arguments: { country: 'vn', query: chosen.question } } })
const sample = result.structuredContent
if (!sample || typeof sample.release !== 'string' || !sample.release || !Array.isArray(sample.hits) || sample.hits.length === 0) throw new Error('Search returned no sample')
const meta = { release: sample.release, case_id: chosen.id, generated_at: new Date().toISOString() }
await writeFile(resolve(output, 'sample-result.json'), `${JSON.stringify(sample, null, 2)}\n`)
await writeFile(resolve(output, 'sample-result.meta.json'), `${JSON.stringify(meta, null, 2)}\n`)
