import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'
import { legalCopy } from '../i18n/legal'
import { publicHead } from '../i18n/public-head'

it('offers a support email in the footer', () => {
  expect(readFileSync('app/components/SiteFooter.vue', 'utf8')).toContain('mailto:support@trachuan.vn')
})

it('uses distinct legal copy and localized public metadata', () => {
  expect(readFileSync('app/pages/privacy.vue', 'utf8')).not.toContain('<LegalFacts')
  expect(readFileSync('app/pages/terms.vue', 'utf8')).not.toContain('<LegalFacts')
  expect(readFileSync('app/layouts/default.vue', 'utf8')).toContain('publicHead')
})

it('shows the official image on the Google sign-in link', () => {
  expect(readFileSync('app/pages/login.vue', 'utf8')).toContain('src="/google-g.png"')
})

it('provides every legal section in both languages', () => {
  for (const language of ['vi', 'en'] as const) {
    const copy = legalCopy(language)
    expect(copy.privacy).toHaveLength(14)
    expect(copy.terms).toHaveLength(8)
    for (const section of [...copy.privacy, ...copy.terms]) {
      expect(section[0]).toBeTruthy()
      expect(section[1]).toBeTruthy()
    }
    expect(JSON.stringify(copy)).toContain('support@trachuan.vn')
    expect(JSON.stringify(copy.privacy)).toContain('openid')
    expect(JSON.stringify(copy.privacy)).toContain('Argon2id')
  }
})

it('provides localized metadata for every named public route', () => {
  for (const language of ['vi', 'en'] as const) {
    for (const path of ['/', '/vn/topics', '/vn/topics/example', '/connect', '/sources', '/privacy', '/terms', '/login', '/signup', '/reset']) {
      expect(publicHead(language, path)?.title, path).toBeTruthy()
      expect(publicHead(language, path)?.description, path).toBeTruthy()
    }
  }
})

it('discloses stored Google identity and later email changes', () => {
  expect(legalCopy('vi').privacy[1]?.[1]).toContain('Chúng tôi lưu mã định danh tài khoản Google và địa chỉ email tại thời điểm bạn đăng ký; thay đổi email Google sau đó không được sao chép.')
  expect(legalCopy('en').privacy[1]?.[1]).toContain('We store the Google account identifier and the email address it had when you signed up; later changes to your Google email are not copied.')
})
