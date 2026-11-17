import type { Language } from './index'
import { authMessages } from './auth'

const heads = {
  vi: {
    '/': ['Tra Chuẩn · Tra cứu nguyên văn pháp luật Việt Nam', 'Hỏi bằng tiếng Việt, đọc nguyên văn điều, khoản trong văn bản pháp luật Việt Nam, kèm số hiệu và trạng thái hiệu lực. Dùng trên web hoặc qua MCP.'],
    '/vn/topics': ['Chủ đề pháp luật theo Bộ pháp điển · Tra Chuẩn', 'Duyệt văn bản pháp luật Việt Nam theo cây chủ đề của Bộ pháp điển, kèm số văn bản và số quy định trong từng chủ đề.'],
    '/connect': ['Kết nối trợ lý AI qua MCP · Tra Chuẩn', 'Cách kết nối Claude, ChatGPT, Claude Code và Cursor với Tra Chuẩn qua MCP để tra cứu và trích nguyên văn pháp luật Việt Nam.'],
    '/sources': ['Nguồn dữ liệu và phạm vi · Tra Chuẩn', 'Văn bản lấy từ Cơ sở dữ liệu quốc gia về văn bản pháp luật và Cổng thông tin điện tử Chính phủ; bản phát hành hiện tại và những gì còn thiếu.'],
    '/privacy': ['Quyền riêng tư · Tra Chuẩn', 'Tra Chuẩn lưu gì và không lưu gì: tài khoản, câu hỏi, nhật ký, thời gian lưu và các bên xử lý dữ liệu.'],
    '/terms': ['Điều khoản sử dụng · Tra Chuẩn', 'Tra Chuẩn đưa bằng chứng từ văn bản pháp luật, không phải tư vấn pháp lý. Giới hạn sử dụng, tạm ngưng và xóa tài khoản.'],
    '/login': ['Đăng nhập', 'Đăng nhập Tra Chuẩn để tra cứu pháp luật.'],
    '/login/factor': [authMessages('vi').factorTitle, 'Nhập mã xác minh để đăng nhập Tra Chuẩn.'],
    '/signup': ['Đăng ký', 'Đăng ký tài khoản Tra Chuẩn.'],
    '/reset': ['Đặt lại mật khẩu', 'Yêu cầu liên kết đặt lại mật khẩu Tra Chuẩn.'],
    '/link': ['Đặt mật khẩu', 'Đặt mật khẩu cho tài khoản Tra Chuẩn.'],
  },
  en: {
    '/': ['Tra Chuẩn · Vietnamese law, word for word', 'Ask in Vietnamese and read articles and clauses of Vietnamese legal documents word for word, with document numbers and status. On the web or over MCP.'],
    '/vn/topics': ['Legal topics from the legal code · Tra Chuẩn', 'Browse Vietnamese legal documents by the topic tree of the legal code, with the number of documents and provisions in each topic.'],
    '/connect': ['Connect AI assistants over MCP · Tra Chuẩn', 'How to connect Claude, ChatGPT, Claude Code and Cursor to Tra Chuẩn over MCP to search and quote Vietnamese law word for word.'],
    '/sources': ['Sources and coverage · Tra Chuẩn', 'Documents come from the national legal database and the Government Portal. See the current release, its counts and what is still missing.'],
    '/privacy': ['Privacy · Tra Chuẩn', 'What Tra Chuẩn stores and does not store: accounts, questions, logs, how long data is kept and who processes it.'],
    '/terms': ['Terms of use · Tra Chuẩn', 'Tra Chuẩn gives evidence from legal texts, not legal advice. Usage limits, suspension and account deletion.'],
    '/login': ['Sign in', 'Sign in to Tra Chuẩn to search legal texts.'],
    '/login/factor': [authMessages('en').factorTitle, 'Enter a verification code to sign in to Tra Chuẩn.'],
    '/signup': ['Sign up', 'Create a Tra Chuẩn account.'],
    '/reset': ['Reset password', 'Request a link to reset your Tra Chuẩn password.'],
    '/link': ['Set password', 'Set a password for your Tra Chuẩn account.'],
  },
}
export function publicHead(language: Language, path: string) {
  const key = path.startsWith('/vn/topics/') ? '/vn/topics' : path
  const values = heads[language][key as keyof typeof heads.vi]
  return values ? { title: values[0], description: values[1] } : undefined
}
