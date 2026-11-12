import { expect, it } from 'vitest'
import { matchPage } from '../server/utils/page-route'

it('matches only topic codes in the public grammar and canonicalizes slashes', () => {
  for (const path of ['/vn/topics', '/vn/topics/1', '/vn/topics/999.9999.p1.c1', '/vn/topics/1.1.c2.m3']) {
    expect(matchPage(path)).toEqual({ path, public: true, alias: false })
  }
  for (const path of ['/vn/topics/0', '/vn/topics/01', '/vn/topics/1000', '/vn/topics/1.0', '/vn/topics/1.1.p0', '/vn/topics/1.1.c1.p1', '/vn/topics/%31', '/VN/topics', '/vn/topics/1/extra']) {
    expect(matchPage(path)).toBeNull()
  }
  expect(matchPage('/vn//topics/1/?x=1')).toEqual({ path: '/vn/topics/1', public: true, alias: true })
})
