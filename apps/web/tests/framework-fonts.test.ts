import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'

it('uses the site font tokens for source text and citations', () => {
  for (const path of ['app/components/FrameworkUnits.vue', 'app/pages/frameworks/search.vue']) {
    const source = readFileSync(path, 'utf8')
    expect(source).toMatch(/\.fw-source\s*\{[^}]*font-family:\s*var\(--font-serif\)/)
    expect(source).toMatch(/\.fw-mono\s*\{[^}]*font-family:\s*var\(--font-mono\)/)
  }
})
