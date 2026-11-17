import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'
import { legalCopy } from '../i18n/legal'

it('renders localized page titles and the wordmark without search query text', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => new Response(JSON.stringify(new URL(request.url).pathname === '/api/v1/me' ? { id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' } : { release: 'test', documents: 2, provisions: 3, nodes: 1 }), { headers: { 'content-type': 'application/json' } });
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
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
    ['Tra Chuẩn · Tra cứu nguyên văn pháp luật Việt Nam', 'Kết nối trợ lý AI qua MCP · Tra Chuẩn', 'Nguồn dữ liệu và phạm vi · Tra Chuẩn', 'Quyền riêng tư · Tra Chuẩn', 'Điều khoản sử dụng · Tra Chuẩn', 'Đăng nhập', 'Tài khoản', 'Tra cứu', 'Văn bản'],
    ['Tra Chuẩn · Vietnamese law, word for word', 'Connect AI assistants over MCP · Tra Chuẩn', 'Sources and coverage · Tra Chuẩn', 'Privacy · Tra Chuẩn', 'Terms of use · Tra Chuẩn', 'Sign in', 'Account', 'Search', 'Document'],
  ]
  for (const [index, page] of pages.slice(0, 18).entries()) {
    const title = titles[Math.floor(index / 9)]![index % 9]
    expect(page).toContain(`<title>${index % 9 < 5 ? title : `${title} · Tra Chuẩn`}</title>`)
    expect(page).toMatch(/<a[^>]*class="[^"]*site-logo[^"]*"[^>]*>.*?<span>Tra Chuẩn<\/span><\/a>/)
    expect(page).not.toContain('private-search')
  }
  expect(pages[5]).toContain('<h1>Đăng nhập để tra cứu</h1>')
  expect(pages[14]).toContain('<h1>Sign in to search</h1>')
  expect(pages[3]).toContain('Tra Chuẩn trả về bằng chứng')
  expect(pages[12]).toContain('Tra Chuẩn returns evidence')
  for (const [language, offset] of [['vi', 0], ['en', 9]] as const) {
    const copy = legalCopy(language)
    for (const [heading, text] of copy.privacy) { expect(pages[3 + offset]).toContain(heading); expect(pages[3 + offset]).toContain(text) }
    for (const [heading, text] of copy.terms) { expect(pages[4 + offset]).toContain(heading); expect(pages[4 + offset]).toContain(text) }
    expect(pages[3 + offset]).toContain(copy.updated)
    expect(pages[4 + offset]).toContain(copy.updated)
  }
  expect(pages[18]).toMatch(/^# Tra Chuẩn\n\n> Tra Chuẩn \(trachuan\) provides evidence/)
})
