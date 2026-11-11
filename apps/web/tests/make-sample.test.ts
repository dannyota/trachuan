import { execFile } from 'node:child_process'
import { mkdtemp, readFile, readdir, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { promisify } from 'node:util'
import { expect, it } from 'vitest'

const run = promisify(execFile)
const token = 'dummy-sample-token'
const sample = {
  release: '20990101_1',
  abstain: false,
  documents: { 'vn:test': { status: { current: true }, text_source: 'source_tree', text_complete: true } },
  hits: [{ doc: 'vn:test', citation: 'Điều 1', text: 'Văn bản mẫu.', complete: true }],
}

it.each(['success', 'tool-error', 'rpc-error'])('generates a sample or writes nothing for %s', async (mode) => {
  const output = await mkdtemp(join(tmpdir(), 'trachuan-sample-'))
  const requests: Array<{ method: string, auth?: string, version?: string, name?: string, country?: string }> = []
  const server = createServer(async (req, res) => {
    const chunks: Uint8Array[] = []
    for await (const chunk of req) chunks.push(chunk)
    const body = JSON.parse(Buffer.concat(chunks).toString())
    requests.push({ method: body.method, auth: req.headers.authorization, version: req.headers['mcp-protocol-version'] as string | undefined, name: body.params?.name, country: body.params?.arguments?.country })
    if (body.method === 'notifications/initialized') { res.writeHead(202).end(); return }
    const result = body.method === 'initialize'
      ? { protocolVersion: '2025-06-18', capabilities: {}, serverInfo: { name: 'stub', version: '1' } }
      : mode === 'tool-error' ? { isError: true } : { structuredContent: sample }
    const reply = mode === 'rpc-error' && body.method === 'tools/call'
      ? { jsonrpc: '2.0', id: body.id, error: { code: -32603, message: 'error' } }
      : { jsonrpc: '2.0', id: body.id, result }
    res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(reply))
  })
  try {
    await new Promise<void>(resolveListen => server.listen(0, '127.0.0.1', resolveListen))
    const address = server.address()
    if (!address || typeof address === 'string') throw new Error('Missing stub address')
    const env = { TRACHUAN_URL: `http://127.0.0.1:${address.port}`, TRACHUAN_TOKEN: token, TRACHUAN_SAMPLE_DIR: output }
    let failed = false
    try {
      await run(process.execPath, [resolve('scripts/make-sample.mjs')], { cwd: tmpdir(), env, timeout: 5000 })
    } catch {
      failed = true
    }
    expect(requests.map(request => request.method)).toEqual(['initialize', 'notifications/initialized', 'tools/call'])
    for (const request of requests) {
      expect(request.auth).toBe(`Bearer ${token}`)
      expect(request.version).toBe('2025-06-18')
    }
    expect(requests[2]).toMatchObject({ name: 'search', country: 'vn' })
    if (mode !== 'success') {
      expect(failed).toBe(true)
      expect(await readdir(output)).toEqual([])
      return
    }
    expect(failed).toBe(false)
    expect((await readdir(output)).sort()).toEqual(['sample-result.json', 'sample-result.meta.json'])
    const resultText = await readFile(join(output, 'sample-result.json'), 'utf8')
    const metaText = await readFile(join(output, 'sample-result.meta.json'), 'utf8')
    expect(resultText).not.toContain(token)
    expect(metaText).not.toContain(token)
    expect(JSON.parse(resultText)).toEqual(sample)
    expect(JSON.parse(metaText)).toMatchObject({ release: '20990101_1', case_id: 'atttt-50-2024' })
    const require = createRequire(import.meta.url)
    const spec = require('yaml').parse(await readFile('../../packages/schema/openapi.yaml', 'utf8'))
    const validate = new (require('ajv'))({ strict: false, formats: { int64: true } }).compile({ ...spec.components.schemas.SearchResult, components: spec.components })
    expect(validate(JSON.parse(resultText)), JSON.stringify(validate.errors)).toBe(true)
  } finally {
    await new Promise<void>((resolveClose, reject) => server.close(error => error ? reject(error) : resolveClose()))
    await rm(output, { recursive: true, force: true })
  }
})
