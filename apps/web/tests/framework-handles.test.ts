import { describe, expect, it } from 'vitest'
import { controlHandle, controlPath, frameworkNext, versionPath } from '../app/utils/framework-handles'
import { safeNext } from '../app/api/next'

describe('framework identities', () => {
  it.each(['A 1', 'A (1)', 'A/1', 'A#1', 'A%1', '.', '..'])('round trips citation %s', (citation) => {
    const path = controlPath('fw:fw-alpha:2.1#ignored', citation)
    expect(new URL(path!, 'https://example.test').searchParams.get('citation')).toBe(citation)
    expect(safeNext(path!)).toBe(path)
    expect(controlHandle('fw-alpha', '2.1', citation)).toBe(`fw:fw-alpha:2.1#${Array.from(citation, char => /[A-Za-z0-9.()_~-]/.test(char) ? char : `%${char.charCodeAt(0).toString(16).toUpperCase().padStart(2, '0')}`).join('')}`)
  })
  it('falls back through the version to frameworks', () => {
    const path = controlPath('fw:fw-alpha:2.1', 'A\\1')!
    expect(frameworkNext(path)).toBe(versionPath('fw:fw-alpha:2.1'))
    expect(frameworkNext(`/frameworks/${'a'.repeat(512)}/2.1/control?citation=A`)).toBe('/frameworks')
  })
})

it('keeps a citation after Nuxt normalizes the incoming query', () => {
  const path = '/frameworks/fw-alpha/2.1/control?citation=A+(1)/%23%25'
  expect(frameworkNext(path)).toBe('/frameworks/fw-alpha/2.1/control?citation=A+%281%29%2F%23%25')
})

it.each(['A?1', 'A%3F1'])('keeps a question mark in citation %s', (citation) => {
  const input = `/frameworks/fw-alpha/2.1/control?citation=${citation}`
  const expected = '/frameworks/fw-alpha/2.1/control?citation=A%3F1'
  expect(frameworkNext(input)).toBe(expected)
  expect(safeNext(input)).toBe(expected)
  expect(new URL(frameworkNext(input), 'https://example.test').searchParams.get('citation')).toBe('A?1')
})
