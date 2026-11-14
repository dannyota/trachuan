import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'

it('renders localized titles and descriptions on public routes', () => {
  const script = `
    import { Server } from 'node:http';
    Server.prototype.listen = function () { return this };
    process.env.NITRO_SHUTDOWN_DISABLED = '1';
    globalThis.fetch = async request => {
      const path = new URL(request.url).pathname;
      const node = { code: '1', name: 'Fixture topic', provisions: 4, documents: 2 };
      const data = path.endsWith('/taxonomy/1') ? { node, path: [node], documents: [] } : path.endsWith('/taxonomy') ? { nodes: [node], truncated: false } : { release: 'test', documents: 2, provisions: 3, nodes: 1 };
      return new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' } });
    };
    const app = (await import('./.output/server/chunks/nitro/nitro.mjs')).g();
    const results = [];
    for (const lang of ['vi', 'en']) for (const path of ['/', '/vn/topics', '/vn/topics/1', '/connect', '/sources', '/privacy', '/terms', '/login', '/signup', '/reset']) {
      const response = await app.localFetch(path, { headers: { cookie: 'lang=' + lang } });
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
    ['Find evidence from Vietnamese legal texts.', 'Explore Vietnamese legal texts by topic.', 'Explore Vietnamese legal texts by topic.', 'Connect AI assistants to Vietnamese legal evidence.', 'See the sources and coverage of Vietnamese legal texts.', 'Learn how Tra Chuẩn handles and protects your data.', 'Read the terms for using Tra Chuẩn.', 'Sign in to Tra Chuẩn to search legal texts.', 'Create a Tra Chuẩn account.', 'Request a link to reset your Tra Chuẩn password.'],
  ]
  for (const [index, page] of pages.entries()) {
    const language = Math.floor(index / 10)
    const route = index % 10
    expect(page.status, `page ${index}`).toBe(200)
    expect(page.html).toContain(`<title>${titles[language]![route]} · Tra Chuẩn</title>`)
    expect(page.html).toContain(`<meta name="description" content="${descriptions[language]![route]}">`)
  }
})
