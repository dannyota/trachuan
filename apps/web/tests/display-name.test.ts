import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'
import { legalCopy } from '../i18n/legal'

it('renders localized page titles and the wordmark without search query text', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => new Response(JSON.stringify(new URL(request.url).pathname === '/api/v1/me' ? { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' } : { release: 'test', documents: 2, provisions: 3, nodes: 1 }), { headers: { 'content-type': 'application/json' } });
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const results = [];
    for (const lang of ['vi', 'en']) for (const path of ['/', '/connect', '/sources', '/privacy', '/terms', '/login', '/account', '/vn/search?query=private-search', '/vn/documents/vn:vbpl:1']) {
      const response = await app.localFetch(path, { headers: { cookie: 'session=ok; lang=' + lang } });
      results.push(await response.text());
    }
    results.push(await (await app.localFetch('/llms.txt')).text());
    console.log(JSON.stringify(results));
  `
  const pages: string[] = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  const titles = [
    ['Tra cứu pháp luật', 'Kết nối trợ lý AI', 'Nguồn dữ liệu', 'Quyền riêng tư', 'Điều khoản', 'Đăng nhập', 'Tài khoản', 'Tra cứu', 'Văn bản'],
    ['Legal research', 'Connect AI assistants', 'Sources', 'Privacy', 'Terms', 'Sign in', 'Account', 'Search', 'Document'],
  ]
  for (const [index, page] of pages.slice(0, 18).entries()) {
    expect(page).toContain(`<title>${titles[Math.floor(index / 9)]![index % 9]} · Tra Chuẩn</title>`)
    expect(page).toMatch(/<a[^>]*class="[^"]*site-logo[^"]*"[^>]*>.*?<span>Tra Chuẩn<\/span><\/a>/)
    expect(page).not.toContain('private-search')
  }
  expect(pages[5]).toContain('<h1>Đăng nhập vào Tra Chuẩn</h1>')
  expect(pages[14]).toContain('<h1>Sign in to Tra Chuẩn</h1>')
  expect(pages[3]).toContain('Tra Chuẩn trả về bằng chứng')
  expect(pages[12]).toContain('Tra Chuẩn returns evidence')
  for (const [language, offset] of [['vi', 0], ['en', 9]] as const) {
    const copy = legalCopy(language)
    for (const [heading, text] of copy.privacy) { expect(pages[3 + offset]).toContain(heading); expect(pages[3 + offset]).toContain(text) }
    for (const [heading, text] of copy.terms) { expect(pages[4 + offset]).toContain(heading); expect(pages[4 + offset]).toContain(text) }
    expect(pages[3 + offset]).toContain(copy.updated)
    expect(pages[4 + offset]).toContain(copy.updated)
  }
  expect(pages[18]).toMatch(/^Tra Chuẩn \(trachuan\) provides evidence/)
})
