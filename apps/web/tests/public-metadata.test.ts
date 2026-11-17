import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

it('renders localized titles and descriptions on public routes', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => {
      const path = new URL(request.url).pathname;
      if (path === '/api/v1/me') return new Response(JSON.stringify({ id: 1, email: 'test@example.invalid', created_at: '2026-01-01T00:00:00Z', csrf_token: 'token' }), { headers: { 'content-type': 'application/json' } });
      const node = { code: '1', name: 'Fixture topic', provisions: 4, documents: 2 };
      const data = path.endsWith('/taxonomy/1') ? { node, path: [node], documents: [] } : path.endsWith('/taxonomy') ? { nodes: [node], truncated: false } : { release: 'test', documents: 2, provisions: 3, nodes: 1 };
      return new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' } });
    };
    const { nitroApp: app } = await import('./tests/nitro-app.mjs');
    const results = [];
    for (const lang of ['vi', 'en']) for (const path of ['/', '/vn/topics', '/vn/topics/1', '/connect', '/sources', '/privacy', '/terms', '/login', '/signup', '/reset']) {
      const response = await app.localFetch(path + '?ignored=1', { headers: { cookie: 'lang=' + lang } });
      results.push({ status: response.status, html: await response.text() });
    }
    for (const lang of ['vi', 'en']) for (const path of ['/account', '/vn/search', '/vn/documents/vn:vbpl:1']) {
      const response = await app.localFetch(path, { headers: { cookie: 'session=ok; lang=' + lang } });
      results.push({ status: response.status, html: await response.text() });
    }
    console.log(JSON.stringify(results));
  `
  const pages: { status: number, html: string }[] = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8' }))
  const titles = [
    ['Tra Chuẩn · Tra cứu nguyên văn pháp luật Việt Nam', 'Chủ đề pháp luật theo Bộ pháp điển · Tra Chuẩn', 'Fixture topic · Chủ đề · Tra Chuẩn', 'Kết nối trợ lý AI qua MCP · Tra Chuẩn', 'Nguồn dữ liệu và phạm vi · Tra Chuẩn', 'Quyền riêng tư · Tra Chuẩn', 'Điều khoản sử dụng · Tra Chuẩn', 'Đăng nhập · Tra Chuẩn', 'Đăng ký · Tra Chuẩn', 'Đặt lại mật khẩu · Tra Chuẩn'],
    ['Tra Chuẩn · Vietnamese law, word for word', 'Legal topics from the legal code · Tra Chuẩn', 'Fixture topic · Topics · Tra Chuẩn', 'Connect AI assistants over MCP · Tra Chuẩn', 'Sources and coverage · Tra Chuẩn', 'Privacy · Tra Chuẩn', 'Terms of use · Tra Chuẩn', 'Sign in · Tra Chuẩn', 'Sign up · Tra Chuẩn', 'Reset password · Tra Chuẩn'],
  ]
  const descriptions = [
    ['Hỏi bằng tiếng Việt, đọc nguyên văn điều, khoản trong văn bản pháp luật Việt Nam, kèm số hiệu và trạng thái hiệu lực. Dùng trên web hoặc qua MCP.', 'Duyệt văn bản pháp luật Việt Nam theo cây chủ đề của Bộ pháp điển, kèm số văn bản và số quy định trong từng chủ đề.', 'Văn bản và chủ đề con của “Fixture topic” trong Bộ pháp điển: 2 văn bản, 4 quy định.', 'Cách kết nối Claude, ChatGPT, Claude Code và Cursor với Tra Chuẩn qua MCP để tra cứu và trích nguyên văn pháp luật Việt Nam.', 'Văn bản lấy từ Cơ sở dữ liệu quốc gia về văn bản pháp luật và Cổng thông tin điện tử Chính phủ; bản phát hành hiện tại và những gì còn thiếu.', 'Tra Chuẩn lưu gì và không lưu gì: tài khoản, câu hỏi, nhật ký, thời gian lưu và các bên xử lý dữ liệu.', 'Tra Chuẩn đưa bằng chứng từ văn bản pháp luật, không phải tư vấn pháp lý. Giới hạn sử dụng, tạm ngưng và xóa tài khoản.'],
    ['Ask in Vietnamese and read articles and clauses of Vietnamese legal documents word for word, with document numbers and status. On the web or over MCP.', 'Browse Vietnamese legal documents by the topic tree of the legal code, with the number of documents and provisions in each topic.', 'Documents and subtopics under “Fixture topic” in the legal code: 2 documents, 4 provisions.', 'How to connect Claude, ChatGPT, Claude Code and Cursor to Tra Chuẩn over MCP to search and quote Vietnamese law word for word.', 'Documents come from the national legal database and the Government Portal. See the current release, its counts and what is still missing.', 'What Tra Chuẩn stores and does not store: accounts, questions, logs, how long data is kept and who processes it.', 'Tra Chuẩn gives evidence from legal texts, not legal advice. Usage limits, suspension and account deletion.'],
  ]
  for (const [index, page] of pages.slice(0, 20).entries()) {
    const language = Math.floor(index / 10)
    const route = index % 10
    expect(page.status, `page ${index}`).toBe(200)
    expect(page.html.match(/<title>/g)).toHaveLength(1)
    expect(page.html).toContain(`<title>${titles[language]![route]}</title>`)
    const fallback = { 'og:site_name': 'Tra Chuẩn', 'og:title': route < 7 ? `${titles[language]![route]}` : 'Tra Chuẩn', 'og:locale': language ? 'en_US' : 'vi_VN', 'og:image': 'https://trachuan.vn/og-image.png', 'og:image:width': '1200', 'og:image:height': '630', 'og:image:alt': 'Tra Chuẩn' }
    for (const [property, content] of Object.entries(fallback)) {
      expect(page.html).toContain(`<meta property="${property}" content="${content}">`)
      expect(page.html.split(`property="${property}"`)).toHaveLength(2)
    }
    expect(page.html.match(/<meta name="twitter:card"/g)).toHaveLength(1)
    expect(page.html).toContain('<meta name="twitter:card" content="summary_large_image">')
    if (route < 7) {
      expect(page.html.match(/<meta name="description"/g)).toHaveLength(1)
      expect(page.html).toContain(`<meta name="description" content="${descriptions[language]![route]}">`)
      const paths = ['/', '/vn/topics', '/vn/topics/1', '/connect', '/sources', '/privacy', '/terms']
      const canonical = `https://trachuan.vn${paths[route]}`
      expect(page.html.match(/<link rel="canonical"/g)).toHaveLength(1)
      expect(page.html).toContain(`<link rel="canonical" href="${canonical}">`)
      for (const [property, content] of Object.entries({ 'og:type': 'website', 'og:site_name': 'Tra Chuẩn', 'og:title': `${titles[language]![route]}`, 'og:description': descriptions[language]![route], 'og:url': canonical, 'og:locale': language ? 'en_US' : 'vi_VN', 'og:image': 'https://trachuan.vn/og-image.png', 'og:image:width': '1200', 'og:image:height': '630', 'og:image:alt': 'Tra Chuẩn' })) {
        expect(page.html).toContain(`<meta property="${property}" content="${content}">`)
        expect(page.html.split(`property="${property}"`)).toHaveLength(2)
      }
      expect(page.html).toContain('<meta name="twitter:card" content="summary_large_image">')
      expect(`${titles[language]![route]}`.normalize('NFC').length).toBeLessThanOrEqual(60)
      expect(descriptions[language]![route]!.normalize('NFC').length).toBeLessThanOrEqual(160)
    } else {
      expect(page.html).not.toContain('rel="canonical"')
      expect(page.html).not.toContain('name="description"')
      expect(page.html).not.toContain('property="og:description"')
      expect(page.html).not.toContain('property="og:url"')
    }
  }
  for (const [index, page] of pages.slice(20).entries()) {
    expect(page.status).toBe(200)
    for (const [property, content] of Object.entries({ 'og:site_name': 'Tra Chuẩn', 'og:title': 'Tra Chuẩn', 'og:locale': index < 3 ? 'vi_VN' : 'en_US', 'og:image': 'https://trachuan.vn/og-image.png', 'og:image:width': '1200', 'og:image:height': '630', 'og:image:alt': 'Tra Chuẩn' })) {
      expect(page.html).toContain(`<meta property="${property}" content="${content}">`)
      expect(page.html.split(`property="${property}"`)).toHaveLength(2)
    }
    expect(page.html.match(/<meta name="twitter:card"/g)).toHaveLength(1)
    expect(page.html).toContain('<meta name="twitter:card" content="summary_large_image">')
    expect(page.html).not.toContain('name="description"')
    expect(page.html).not.toContain('property="og:description"')
    expect(page.html).not.toContain('property="og:url"')
    expect(page.html).not.toContain('rel="canonical"')
  }
})
