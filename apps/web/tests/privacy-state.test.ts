import { describe, expect, it } from 'vitest'
import { containsPrivateValue } from '../e2e/privacy-state'

describe('private state detection', () => {
  const query = 'private query\nsecond line'
  it('finds a multiline value in nested state and object keys', () => {
    expect(containsPrivateValue({ storage: { key: `prefix ${query}` } }, query)).toBe(true)
    expect(containsPrivateValue({ [query]: 'value' }, query)).toBe(true)
  })
  it('finds values escaped in embedded JSON', () => {
    expect(containsPrivateValue({ payload: JSON.stringify({ query }) }, query)).toBe(true)
  })
  it('accepts unrelated state', () => {
    expect(containsPrivateValue({ local: { key: 'other' }, history: [null, 1] }, query)).toBe(false)
  })
})
