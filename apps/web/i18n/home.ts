import type { Language } from './index'

export const en = {
  eyebrow: 'Vietnamese law, backed by sources', title: 'Find the right article and clause, with its legal status.',
  intro: 'Each result is the original text from the source document, with its number, issue date and a link to the Vietnam National Database of Legal Documents. Tra Chuẩn does not write answers.',
  query: 'Question or document number', placeholder: 'For example: minimum password length for online banking', submit: 'Search', inForce: 'Only documents in force', signIn: 'Sign in with Google to search.', suggestions: 'Suggestions:',
  suggestion1: 'biometric authentication for money transfers', suggestion2: 'level 3 information systems', suggestion3: 'delete personal data on request', suggestion4: '50/2024/TT-NHNN',
  sampleTitle: 'What a result looks like', sampleText: 'Find the exact clause that answers a question, not the whole document. The stamp shows whether the document is in force, partly in force or expired on the search date. Warnings appear on the clause when another document amends or repeals it.',
  check1: 'Original text, without paraphrasing', check2: 'Copyable citation: Điều 11, Khoản 1, Thông tư 50/2024/TT-NHNN', check3: 'Link to the original on vbpl.vn',
  scope: 'Focused on banking, finance, information technology and personal data.', scopeLink: 'See coverage and what is missing →',
  connectTitle: 'Use it in your AI assistant', connectText: 'Connect through MCP so Claude or ChatGPT can find and cite the original text.', address: 'MCP address', copy: 'Copy', claude: 'Add a custom connector and sign in with Google once.', chat: 'Enable developer mode and add an MCP connector.', code: 'Add the server with this command.', connectLink: 'Connection instructions →',
}
export const vi: Record<keyof typeof en, string> = {
  eyebrow: 'Pháp luật Việt Nam, có dẫn chứng', title: 'Tra cứu đúng điều, đúng khoản, kèm trạng thái hiệu lực.',
  intro: 'Mỗi kết quả là nguyên văn quy định từ văn bản gốc, có số hiệu, ngày ban hành và đường dẫn tới Cơ sở dữ liệu quốc gia về văn bản pháp luật. Tra Chuẩn không tự viết câu trả lời.',
  query: 'Câu hỏi hoặc số hiệu văn bản', placeholder: 'Ví dụ: độ dài tối thiểu của mật khẩu ngân hàng trực tuyến', submit: 'Tra cứu', inForce: 'Chỉ văn bản còn hiệu lực', signIn: 'Cần đăng nhập Google để tra cứu.', suggestions: 'Gợi ý:',
  suggestion1: 'xác thực sinh trắc học khi chuyển tiền', suggestion2: 'hệ thống thông tin cấp độ 3', suggestion3: 'xóa dữ liệu cá nhân theo yêu cầu', suggestion4: '50/2024/TT-NHNN',
  sampleTitle: 'Một kết quả trông như thế này', sampleText: 'Trích đúng khoản trả lời câu hỏi, không phải cả văn bản. Con dấu cho biết văn bản còn hiệu lực, hết hiệu lực một phần hay đã hết hiệu lực vào ngày tra cứu. Cảnh báo hiện ngay trên khoản khi một văn bản khác đã sửa đổi hoặc bãi bỏ nó.',
  check1: 'Nguyên văn, không diễn giải lại', check2: 'Trích dẫn sao chép được: Điều 11, Khoản 1, Thông tư 50/2024/TT-NHNN', check3: 'Liên kết về bản gốc trên vbpl.vn',
  scope: 'Tập trung vào ngân hàng, tài chính, công nghệ thông tin và dữ liệu cá nhân.', scopeLink: 'Xem phạm vi và những gì còn thiếu →',
  connectTitle: 'Dùng trong trợ lý AI của bạn', connectText: 'Kết nối qua MCP để Claude hoặc ChatGPT tra cứu và trích dẫn nguyên văn, thay vì đoán.', address: 'Địa chỉ MCP', copy: 'Sao chép', claude: 'Thêm connector tùy chỉnh, đăng nhập Google một lần.', chat: 'Bật chế độ nhà phát triển, thêm connector MCP.', code: 'Thêm máy chủ bằng lệnh này.', connectLink: 'Hướng dẫn kết nối →',
}
export function homeMessages(language: Language): Record<keyof typeof en, string> {
  return language === 'en' ? en : vi
}
