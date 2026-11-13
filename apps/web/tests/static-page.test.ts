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

it('keeps static page colors equal to tokens in each theme', () => {
  const tokens = readFileSync('app/assets/tokens.css', 'utf8').split('@media (prefers-color-scheme: dark)')
  const css = readFileSync('public/static-page.css', 'utf8').split('@media (prefers-color-scheme: dark)')
  expect(css).toHaveLength(2)
  for (const theme of [0, 1]) {
    for (const name of ['page', 'ink', 'muted', 'primary', 'seal']) {
      const pattern = new RegExp(`--color-${name}:\\s*([^;]+);`)
      const expected = tokens[theme]?.match(pattern)?.[1]
      expect(expected).toBeDefined()
      expect(css[theme]?.match(pattern)?.[1]).toBe(expected)
    }
  }
})

it('keeps static page fonts equal to the light tokens', () => {
  const tokens = readFileSync('app/assets/tokens.css', 'utf8').split('@media')[0]!
  const css = readFileSync('public/static-page.css', 'utf8')
  for (const name of ['sans', 'serif']) {
    const pattern = new RegExp(`--font-${name}:\\s*([^;]+);`)
    expect(css.match(pattern)?.[1]).toBe(tokens.match(pattern)?.[1])
  }
})
