import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { expect, it } from 'vitest'

const require = createRequire(import.meta.url)
const Ajv = require('ajv')
const yaml = require('yaml')

it('matches the OpenAPI search result and records its release and case', () => {
  const spec = yaml.parse(readFileSync('../../packages/schema/openapi.yaml', 'utf8'))
  const validate = new Ajv({ strict: false, formats: { int64: true } }).compile({ ...spec.components.schemas.SearchResult, components: spec.components })
  const sample = JSON.parse(readFileSync('app/data/sample-result.json', 'utf8'))
  const meta = JSON.parse(readFileSync('app/data/sample-result.meta.json', 'utf8'))
  expect(validate(sample), JSON.stringify(validate.errors)).toBe(true)
  expect(sample.hits?.length).toBeGreaterThan(0)
  expect(meta.case_id).toBeTruthy()
  expect(meta.release).toBeTruthy()
  expect(sample.release).toBe(meta.release)
  expect(Date.parse(meta.generated_at)).not.toBeNaN()
  if (process.env.SAMPLE_REQUIRE_REAL === '1') expect(meta.release).not.toBe('fixture')
})
