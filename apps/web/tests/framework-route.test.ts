import { expect, it } from 'vitest'
import { matchPage } from '../server/utils/page-route'

it.each(['/frameworks', '/frameworks/search', '/frameworks/fw-alpha/2.1', '/frameworks/fw-alpha/2.1/control?citation=A'])('admits the private route %s', (path) => {
  expect(matchPage(path)?.public).toBe(false)
})
