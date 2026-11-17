import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'

it('keeps home copy free of em dashes, arrows and emoji', () => {
  for (const path of ['i18n/home.ts', 'i18n/search.ts', 'i18n/reader.ts', 'i18n/shell.ts', 'i18n/pages.ts', 'i18n/auth.ts', 'i18n/account.ts', 'i18n/vi.ts', 'i18n/en.ts', 'server/utils/not-found.ts']) {
    expect(readFileSync(path, 'utf8'), path).not.toMatch(/[\u2014\u2190-\u21ff\p{Extended_Pictographic}]/u)
  }
})
