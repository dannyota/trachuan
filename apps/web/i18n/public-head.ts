import type { Language } from './index'

const heads = {
  vi: {
    '/': ['Trang chủ', 'Tra cứu bằng chứng từ văn bản pháp luật Việt Nam.'],
    '/vn/topics': ['Chủ đề', 'Khám phá văn bản pháp luật Việt Nam theo chủ đề.'],
    '/connect': ['Kết nối trợ lý AI', 'Kết nối trợ lý AI với bằng chứng pháp luật Việt Nam.'],
    '/sources': ['Nguồn dữ liệu', 'Xem nguồn dữ liệu và phạm vi văn bản pháp luật Việt Nam.'],
    '/privacy': ['Quyền riêng tư', 'Tìm hiểu cách Tra Chuẩn xử lý và bảo vệ dữ liệu của bạn.'],
    '/terms': ['Điều khoản', 'Đọc các điều khoản sử dụng Tra Chuẩn.'],
    '/login': ['Đăng nhập', 'Đăng nhập Tra Chuẩn để tra cứu pháp luật.'],
    '/signup': ['Đăng ký', 'Đăng ký tài khoản Tra Chuẩn.'],
    '/reset': ['Đặt lại mật khẩu', 'Yêu cầu liên kết đặt lại mật khẩu Tra Chuẩn.'],
  },
  en: {
    '/': ['Home', 'Find evidence from Vietnamese legal texts.'],
    '/vn/topics': ['Topics', 'Explore Vietnamese legal texts by topic.'],
    '/connect': ['Connect AI assistants', 'Connect AI assistants to Vietnamese legal evidence.'],
    '/sources': ['Sources', 'See the sources and coverage of Vietnamese legal texts.'],
    '/privacy': ['Privacy', 'Learn how Tra Chuẩn handles and protects your data.'],
    '/terms': ['Terms', 'Read the terms for using Tra Chuẩn.'],
    '/login': ['Sign in', 'Sign in to Tra Chuẩn to search legal texts.'],
    '/signup': ['Sign up', 'Create a Tra Chuẩn account.'],
    '/reset': ['Reset password', 'Request a link to reset your Tra Chuẩn password.'],
  },
}
export function publicHead(language: Language, path: string) {
  const key = path.startsWith('/vn/topics/') ? '/vn/topics' : path
  const values = heads[language][key as keyof typeof heads.vi]
  return values ? { title: values[0], description: values[1] } : undefined
}
