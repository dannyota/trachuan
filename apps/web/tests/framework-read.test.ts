import { expect, it } from 'vitest'
import { frameworkItemError } from '../app/utils/framework-read'

it('recognizes errors requiring a restart or a fixed 404', () => {
  expect(frameworkItemError('invalid_next')).toBe('restart')
  expect(frameworkItemError('release_expired')).toBe('restart')
  expect(frameworkItemError('not_found')).toBe('not-found')
  expect(frameworkItemError('invalid_handle')).toBe('not-found')
  expect(frameworkItemError('unit_too_large')).toBe('unit_too_large')
})
