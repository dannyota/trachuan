import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { describe, expect, it } from 'vitest'
import { validityInk, readerLink, resultHits } from '../app/utils/search-results'

const require = createRequire(import.meta.url)
const Ajv = require('ajv')
const yaml = require('yaml')
const spec = yaml.parse(readFileSync('../../packages/schema/openapi.yaml', 'utf8'))
const validate = new Ajv({ strict: false, formats: { int64: true, date: true } }).compile({ ...spec.components.schemas.SearchResult, components: spec.components })
const document = { status: { class: 'partial', current: true }, text_source: 'source_tree', text_complete: true }
const hit = { handle: 'vn:1#dieu-1', doc: 'vn:1', citation: 'Điều 1', complete: true }
const compact = { release: 'r1', abstain: false, documents: { 'vn:1': document }, hits: [{ ...hit, snippet: 'text' }], gaps: [{ kind: 'missing', message: 'gap' }] }
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
    expect(resultHits({ ...compact, abstain: true })).toHaveLength(0)
    expect(readerLink(hit, 'r1')).toBe('/vn/documents/vn%3A1?release=r1#dieu-1')
    expect(readerLink({ ...hit, handle: 'vn:1' }, 'r1')).toBe('/vn/documents/vn%3A1?release=r1')
  })
})
