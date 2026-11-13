import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { describe, expect, it } from 'vitest'
import { validityInk, readerLink, resultHits, appendSearchPage } from '../app/utils/search-results'

const require = createRequire(import.meta.url)
const Ajv = require('ajv')
const yaml = require('yaml')
const spec = yaml.parse(readFileSync('../../packages/schema/openapi.yaml', 'utf8'))
const validate = new Ajv({ strict: false, formats: { int64: true, date: true } }).compile({ ...spec.components.schemas.SearchResult, components: spec.components })
const document = { status: { class: 'partial', current: true }, text_source: 'source_tree', text_complete: true }
const hit = { handle: 'vn:vbpl:1#dieu-1', doc: 'vn:vbpl:1', citation: 'Điều 1', complete: true }
const compact = { release: 'r1', abstain: false, documents: { 'vn:vbpl:1': document }, hits: [{ ...hit, snippet: 'text' }], gaps: [{ kind: 'missing', message: 'gap' }] }
const full = { ...compact, hits: [{ ...hit, text: '<text>', relations: [{ kind: 'cites', complete: true, source_resolved: true, target_resolved: true }] }] }

describe('search results', () => {
  it('accepts compact and full contract fixtures', () => {
    for (const fixture of [compact, full, { release: 'r1', abstain: true }]) expect(validate(fixture), JSON.stringify(validate.errors)).toBe(true)
  })
  it('maps known and unknown status classes without guessing', () => {
    expect(['in_force', 'partial', 'not_yet', 'expired', 'unexpected'].map(validityInk)).toEqual(['in-force', 'partial', 'not-yet', 'expired', 'neutral'])
  })
  it('keeps both groups and pins reader links', () => {
    expect(resultHits({ ...compact, non_current: [hit] })).toHaveLength(2)
    expect(resultHits({ ...compact, abstain: true, reason: 'low_confidence' })).toHaveLength(1)
    expect(readerLink(hit, 'r1')).toBe('/vn/documents/vn:vbpl:1?release=r1#dieu-1')
    expect(readerLink(hit, '')).toBe('/vn/documents/vn:vbpl:1?release=#dieu-1')
    expect(readerLink({ ...hit, handle: 'vn:vbpl:1' }, 'r1')).toBe('/vn/documents/vn:vbpl:1?release=r1')
  })
})

it('rejects invalid document keys in reader links and preserves safe anchors', () => {
  for (const doc of ['vn:1', 'vn:vbpl:a%2Fb', 'vn:VBPL:abc', 'vn:vbpl:..:x', '../x', 'vn:vbpl:']) {
    expect(readerLink({ ...hit, doc }, 'r1')).toBeNull()
  }
  expect(readerLink({ ...hit, handle: 'vn:vbpl:1#dieu-1/khoan-1' }, 'r1')).toBe('/vn/documents/vn:vbpl:1?release=r1#dieu-1/khoan-1')
})

it('admits only document keys matching the reader link grammar', async () => {
  const { matchPage } = await import('../server/utils/page-route')
  for (const doc of ['vn:vbpl:a-1', 'vn:law:A._9']) expect(matchPage(`/vn/documents/${doc}`)).not.toBeNull()
  for (const doc of ['vn:1', 'vn:VBPL:abc', 'vn:vbpl:a%2Fb', 'vn:vbpl:', 'vn:vbpl:a:b']) expect(matchPage(`/vn/documents/${doc}`)).toBeNull()
})

it('takes a generated reader link directly to the signed-in guard', async () => {
  const { execFileSync } = await import('node:child_process')
  const link = readerLink(hit, 'r1')
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async () => new Response(JSON.stringify({ error: { code: 'unauthenticated', message: 'sign in' } }), { status: 401, headers: { 'content-type': 'application/json' } });
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const response = await app.localFetch(process.argv[1]);
    console.log(JSON.stringify({ status: response.status, location: response.headers.get('location') }));
  `
  const output = execFileSync(process.execPath, ['--input-type=module', '-e', script, link!], { encoding: 'utf8' })
  expect(JSON.parse(output)).toMatchObject({ status: 302, location: expect.stringContaining('/api/v1/auth/google/start?next=') })
})

it('appends each page to its own group and merges reference maps', () => {
  const old = { ...compact, hits: [hit], non_current: [{ ...hit, citation: 'Old' }] }
  const page = { ...compact, hits: [{ ...hit, citation: 'Next' }], non_current: [{ ...hit, citation: 'Past' }], nodes: { topic: { name: 'Topic', level: 1 } } }
  const merged = appendSearchPage(old, page)
  expect(merged.hits?.map(item => item.citation)).toEqual(['Điều 1', 'Next'])
  expect(merged.non_current?.map(item => item.citation)).toEqual(['Old', 'Past'])
  expect(merged.nodes?.topic?.name).toBe('Topic')
})

it('links to the current reader release when no release is supplied', () => {
  expect(readerLink(hit)).toBe('/vn/documents/vn:vbpl:1#dieu-1')
  expect(readerLink({ ...hit, handle: 'vn:vbpl:1' })).toBe('/vn/documents/vn:vbpl:1')
})
