import { expect, it } from 'vitest'
import { createAdmission } from '../server/utils/public-admission'
import { matchPage } from '../server/utils/page-route'

it('admits summary and sitemap children as exact public routes', () => {
  for (const path of ['/vn/documents/vn:vbpl:1/summary', '/sitemap-pages.xml', '/sitemap-documents.xml']) {
    expect(matchPage(path)).toEqual({ path, public: true, alias: false })
  }
  expect(matchPage('/vn/documents/vn:vbpl:1')?.public).toBe(false)
  expect(matchPage('/vn/documents/invalid/summary')).toBeNull()
  expect(matchPage('/vn/documents/vn:vbpl:1/summary/extra')).toBeNull()
})

it('reserves two of four render slots for other pages and releases once', () => {
  const admission = createAdmission(() => 0)
  const first = admission.render(true)
  const second = admission.render(true)
  expect(first).not.toBeNull()
  expect(second).not.toBeNull()
  expect(admission.render(true)).toBeNull()
  const other = admission.render()
  expect(other).not.toBeNull()
  expect(admission.render()).not.toBeNull()
  expect(admission.render()).toBeNull()
  first?.()
  first?.()
  expect(admission.render(true)).not.toBeNull()
  expect(admission.render()).toBeNull()
})
