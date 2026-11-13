import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'
import { notFoundDocument } from '../server/utils/not-found'

it('selects a fixed 404 document in the requested language', () => {
  expect(notFoundDocument('vi')).toContain('Không tìm thấy trang')
  expect(notFoundDocument('en')).toContain('Page not found')
  for (const language of ['vi', 'en'] as const) {
    const html = notFoundDocument(language)
    expect(html).toContain('href="/static-page.css"')
    expect(html).toContain('viewBox="0 0 120 120"')
    expect(html).toContain('r="52"')
    for (const path of ['/', '/vn/search', '/vn/topics']) expect(html).toContain(`href="${path}"`)
  }
})

it('keeps static page colors and fonts equal to the light tokens', () => {
  const tokens = readFileSync('app/assets/tokens.css', 'utf8').split('@media')[0]!
  const css = readFileSync('public/static-page.css', 'utf8')
  const values = [...css.matchAll(/(--(?:color|font)-[\w-]+):\s*([^;]+);/g)]
  expect(values.length).toBeGreaterThan(0)
  for (const [, name, value] of values) expect(tokens).toContain(`${name}: ${value};`)
})
