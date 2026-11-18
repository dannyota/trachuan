import type { Language } from './index'

export const en = {
  title: 'Ask in Vietnamese. Read the law in its own words.',
  intro: "Hi, I'm Tra Chuẩn. I don't write answers. I find passages in the source documents and show them word for word, with the document number and the status recorded in the dataset.",
  query: 'Question or document number', placeholder: 'For example: minimum password length for online banking', submit: 'Search', inForce: 'Only documents in force', signIn: 'Sign in to search. I never write your questions to disk or logs.', suggestions: 'Suggestions:',
  sampleTitle: 'A real result from the {date} release.', sampleText: "Each result is a passage from the source document, not the whole document. The stamp is the document's status as recorded in Tra Chuẩn's dataset, with its stated basis; on this sample, the line beside the stamp gives the date the status applies to. A warning appears right on the clause when another document has amended or repealed it.",
  check1: 'The original text, never paraphrased.', check2: 'Copy the citation in one click: {citation}', check3: 'A link to the original, when the source publishes one.',
  scope: 'I focus on banking, finance, IT and personal data, so not every document is here yet.', scopeLink: "See what's missing",
  connectTitle: 'Use Tra Chuẩn from your AI assistant.', connectText: 'Connect Claude, ChatGPT or another MCP-capable assistant. It can search, read the original text and quote it with the document number. You grant read-only access and can revoke it any time.', address: 'MCP address', copy: 'Copy', claude: 'Add a custom connector and sign in once.', chat: 'Enable developer mode and add an MCP connector.', code: 'Add the server with this command.', connectLink: 'See how to connect',
  makerTitle: "Hi, I'm Danny.", makerText: "I once asked an AI assistant about the law. It answered with total confidence and couldn't point to a single clause. So I built a place that shows only the original text and lets you check it for yourself. The site is new and doesn't hold every document yet; the sources page lists what's missing.", makerSearch: 'Search', makerResume: 'See my resume', makerFeedback: 'Send feedback', makerAlt: 'Drawn portrait of Danny, with his wife and child in a heart-shaped frame',
}
export const vi: Record<keyof typeof en, string> = {
  title: 'Hỏi bằng tiếng Việt, đọc nguyên văn từ văn bản gốc.',
  intro: 'Chào bạn, mình là Tra Chuẩn. Mình không viết câu trả lời. Mình tìm các đoạn trong văn bản gốc và đưa nguyên văn, kèm số hiệu và trạng thái hiệu lực ghi trong dữ liệu.',
  query: 'Câu hỏi hoặc số hiệu văn bản', placeholder: 'Ví dụ: độ dài tối thiểu của mật khẩu ngân hàng trực tuyến', submit: 'Tra cứu', inForce: 'Chỉ văn bản còn hiệu lực', signIn: 'Bạn cần đăng nhập để tra cứu. Mình không ghi câu hỏi vào ổ đĩa hay nhật ký.', suggestions: 'Gợi ý:',
  sampleTitle: 'Một kết quả thật, từ bản phát hành ngày {date}.', sampleText: 'Mỗi kết quả là một đoạn trích từ văn bản gốc, không phải cả văn bản. Con dấu là trạng thái hiệu lực ghi trong dữ liệu của Tra Chuẩn, kèm căn cứ. Ở mẫu này, dòng cạnh con dấu ghi trạng thái tính đến ngày nào. Khi một văn bản khác đã sửa đổi hoặc bãi bỏ một khoản, cảnh báo hiện ngay trên khoản đó.',
  check1: 'Nguyên văn, không diễn giải.', check2: 'Bấm một lần để sao chép trích dẫn: {citation}', check3: 'Liên kết về bản gốc, khi nguồn có công bố.',
  scope: 'Mình tập trung vào ngân hàng, tài chính, công nghệ thông tin và dữ liệu cá nhân, nên chưa có mọi văn bản.', scopeLink: 'Xem những gì còn thiếu',
  connectTitle: 'Dùng Tra Chuẩn trong trợ lý AI.', connectText: 'Kết nối Claude, ChatGPT hoặc trợ lý khác hỗ trợ MCP. Trợ lý có thể tra cứu, đọc nguyên văn và trích kèm số hiệu văn bản. Bạn cấp quyền chỉ đọc và thu hồi lúc nào cũng được.', address: 'Địa chỉ MCP', copy: 'Sao chép', claude: 'Thêm connector tùy chỉnh, đăng nhập một lần.', chat: 'Bật chế độ nhà phát triển, thêm connector MCP.', code: 'Thêm máy chủ bằng lệnh này.', connectLink: 'Xem cách kết nối',
  makerTitle: 'Chào bạn, mình là Danny.', makerText: 'Mình từng hỏi trợ lý AI về luật và nhận được câu trả lời rất tự tin, nhưng không chỉ ra được điều khoản nào. Nên mình làm một chỗ chỉ đưa nguyên văn, để bạn tự kiểm tra được. Trang còn mới và chưa đủ mọi văn bản; những gì còn thiếu mình ghi ở trang Nguồn dữ liệu.', makerSearch: 'Tra cứu', makerResume: 'Xem CV của mình', makerFeedback: 'Gửi góp ý', makerAlt: 'Tranh vẽ chân dung Danny, bên cạnh là vợ và con anh trong khung hình trái tim',
}
export function homeMessages(language: Language): Record<keyof typeof en, string> {
  return language === 'en' ? en : vi
}

export const homeSuggestions = ['xác thực sinh trắc học khi chuyển tiền', 'hệ thống thông tin cấp độ 3', 'xóa dữ liệu cá nhân theo yêu cầu', '50/2024/TT-NHNN']
