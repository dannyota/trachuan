import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'

it('keeps home copy free of em dashes, arrows and emoji', () => {
  const copy = readFileSync('i18n/home.ts', 'utf8')
  expect(copy).not.toMatch(/[\u2014\u2190-\u21ff\p{Extended_Pictographic}]/u)
})
