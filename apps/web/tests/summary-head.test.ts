import { expect, it } from 'vitest'
import { summarySeo } from '../app/utils/summary-seo'
import { summaryGraph } from '../app/utils/structured-data'
import { publicSummary } from '../e2e/summary-data'

it('uses the document identity for the title and cuts normalized descriptions at word boundaries', () => {
  const document = { ...publicSummary.document, title: '  Mẫu\n  văn bản '.repeat(30) }
  const seo = summarySeo(document)
  expect(seo.title).toBe('Luật 1/2099/QH · Tra Chuẩn')
  expect(seo.description.length).toBeLessThan(160)
  expect(seo.description).toMatch(/(?:Mẫu|văn|bản)$/)
  expect(seo.description).not.toContain('  ')
  expect(summarySeo({ ...document, type: 'Loại văn bản '.repeat(10) }).title).toBe(document.number)
})

it('limits legislation properties to the visible summary identity and https source', () => {
  const graph = summaryGraph('https://trachuan.vn', publicSummary)
  expect(Object.keys(graph[0]!).sort()).toEqual(['@type', 'inLanguage', 'legislationDate', 'legislationIdentifier', 'legislationJurisdiction', 'legislationPassedBy', 'legislationType', 'name', 'sameAs', 'url'])
  expect(graph[0]?.sameAs).toBe(publicSummary.document.source_url)
  expect(summaryGraph('https://trachuan.vn', { ...publicSummary, document: { ...publicSummary.document, issued: null, issuer: '', source_url: 'http://example.invalid' } })[0]).not.toHaveProperty('sameAs')
})
