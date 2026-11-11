import { expect, it } from 'vitest'
import { getCoverage } from '../app/api/coverage'

it('returns status fields and treats errors as unavailable', async () => {
  const status = { release: '20260101_1', documents: 12, provisions: 34, nodes: 2, gaps: { missing: 3 }, newest_source_date: '2026-01-01' }
  expect(await getCoverage(async () => ({ data: status }))).toEqual(status)
  expect(await getCoverage(async () => ({ error: {} }))).toBeNull()
  expect(await getCoverage(async () => { throw new Error('offline') })).toBeNull()
})
