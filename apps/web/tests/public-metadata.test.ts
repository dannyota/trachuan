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
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
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
    ['Tra cứu pháp luật', 'Chủ đề', 'Chủ đề', 'Kết nối trợ lý AI', 'Nguồn dữ liệu', 'Quyền riêng tư', 'Điều khoản', 'Đăng nhập', 'Đăng ký', 'Đặt lại mật khẩu'],
    ['Legal research', 'Topics', 'Topics', 'Connect AI assistants', 'Sources', 'Privacy', 'Terms', 'Sign in', 'Sign up', 'Reset password'],
  ]
  const descriptions = [
    ['Tra cứu bằng chứng từ văn bản pháp luật Việt Nam.', 'Khám phá văn bản pháp luật Việt Nam theo chủ đề.', 'Khám phá văn bản pháp luật Việt Nam theo chủ đề.', 'Kết nối trợ lý AI với bằng chứng pháp luật Việt Nam.', 'Xem nguồn dữ liệu và phạm vi văn bản pháp luật Việt Nam.', 'Tìm hiểu cách Tra Chuẩn xử lý và bảo vệ dữ liệu của bạn.', 'Đọc các điều khoản sử dụng Tra Chuẩn.', 'Đăng nhập Tra Chuẩn để tra cứu pháp luật.', 'Đăng ký tài khoản Tra Chuẩn.', 'Yêu cầu liên kết đặt lại mật khẩu Tra Chuẩn.'],
    ['Find evidence from Vietnamese legal texts.', 'Explore Vietnamese legal texts by topic.', 'Explore Vietnamese legal texts by topic.', 'Connect AI assistants to Vietnamese legal evidence.', 'See the sources and coverage of Vietnamese legal texts.', 'Learn how Tra Chuẩn handles and protects your data.', 'Read the terms for using Tra Chuẩn.', 'Sign in to search to search legal texts.', 'Create a Tra Chuẩn account.', 'Request a link to reset your Tra Chuẩn password.'],
  ]
  for (const [index, page] of pages.slice(0, 20).entries()) {
    const language = Math.floor(index / 10)
    const route = index % 10
    expect(page.status, `page ${index}`).toBe(200)
    expect(page.html.match(/<title>/g)).toHaveLength(1)
    expect(page.html).toContain(`<title>${titles[language]![route]} · Tra Chuẩn</title>`)
    const fallback = { 'og:site_name': 'Tra Chuẩn', 'og:title': route < 7 ? `${titles[language]![route]} · Tra Chuẩn` : 'Tra Chuẩn', 'og:locale': language ? 'en_US' : 'vi_VN', 'og:image': 'https://trachuan.vn/og-image.png', 'og:image:width': '1200', 'og:image:height': '630', 'og:image:alt': 'Tra Chuẩn' }
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
      for (const [property, content] of Object.entries({ 'og:type': 'website', 'og:site_name': 'Tra Chuẩn', 'og:title': `${titles[language]![route]} · Tra Chuẩn`, 'og:description': descriptions[language]![route], 'og:url': canonical, 'og:locale': language ? 'en_US' : 'vi_VN', 'og:image': 'https://trachuan.vn/og-image.png', 'og:image:width': '1200', 'og:image:height': '630', 'og:image:alt': 'Tra Chuẩn' })) {
        expect(page.html).toContain(`<meta property="${property}" content="${content}">`)
        expect(page.html.split(`property="${property}"`)).toHaveLength(2)
      }
      expect(page.html).toContain('<meta name="twitter:card" content="summary_large_image">')
      expect(`${titles[language]![route]} · Tra Chuẩn`.normalize('NFC').length).toBeLessThanOrEqual(60)
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
