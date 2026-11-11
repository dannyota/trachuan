import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'
import { en } from '../i18n/en'
import { vi } from '../i18n/vi'

it('provides both messages for every OpenAPI error code', () => {
  const schema = readFileSync('../../packages/schema/openapi.yaml', 'utf8')
  const block = schema.split('    Error:\n')[1]!.split('    Account:\n')[0]!
  const codes = [...block.matchAll(/^ {14}- ([a-z_]+)$/gm)].map(match => match[1]!)
  expect(codes.length).toBeGreaterThan(0)
  for (const code of codes) {
    expect(en.errors[code as keyof typeof en.errors]).toBeTruthy()
    expect(vi.errors[code as keyof typeof vi.errors]).toBeTruthy()
  }
})

it('uses the generic message for an unknown error', async () => {
  const { errorMessage } = await import('../i18n')
  expect(errorMessage('en', 'unknown_code')).toBe(en.errors.generic)
  expect(errorMessage('vi', 'csrf_rejected')).toBe(vi.errors.csrf_rejected)
})
