import { expect, it } from 'vitest'
import { notFoundDocument } from '../server/utils/not-found'

it('provides a skip link and focusable main on the fixed 404', () => {
  for (const language of ['vi', 'en'] as const) {
    const html = notFoundDocument(language)
    expect(html).toContain('href="#main"')
    expect(html).toContain('<main id="main" tabindex="-1">')
  }
})
