import type { Language } from './index'

export const en = {
  filters: 'Filters', validity: 'Validity', showAll: 'Show all', showLess: 'Show less', close: 'Close filters',
  results: 'results', weak: 'Weak evidence', weakDetail: 'Results are shown, but the match is weak. Check carefully before citing.',
  noEvidence: 'No evidence found', noEvidenceDetail: 'Try different words or search by document number.',
  idle: 'Enter a question or document number to search.', examples: 'Try an example',
  example1: 'What rules apply to online banking passwords?', example2: 'How can personal data be deleted?',
  loading: 'Searching…', rate: 'Too many requests', more: 'Show more results', nonCurrent: 'Documents no longer in force',
  showNonCurrent: 'Show results from documents no longer in force', outside: 'Results outside the filters',
  matched: 'Matched {n} clauses in this article:', expand: 'Show full text', collapse: 'Show less',
  read: 'Read in document', copy: 'Copy citation', warningMore: 'and {n} more warnings', matchedWords: 'Matched words',
}
export const vi: Record<keyof typeof en, string> = {
  filters: 'Bộ lọc', validity: 'Hiệu lực', showAll: 'Xem tất cả', showLess: 'Thu gọn', close: 'Đóng bộ lọc',
  results: 'kết quả', weak: 'Bằng chứng yếu', weakDetail: 'Kết quả vẫn hiện, nhưng độ khớp thấp. Kiểm tra kỹ trước khi dẫn chiếu.',
  noEvidence: 'Không tìm thấy bằng chứng', noEvidenceDetail: 'Thử diễn đạt khác, hoặc tìm theo số hiệu văn bản.',
  idle: 'Nhập câu hỏi hoặc số hiệu văn bản để tra cứu.', examples: 'Thử câu hỏi gợi ý',
  example1: 'Quy định về mật khẩu ngân hàng trực tuyến là gì?', example2: 'Khi nào được xóa dữ liệu cá nhân?',
  loading: 'Đang tra cứu…', rate: 'Quá nhiều yêu cầu', more: 'Xem thêm kết quả', nonCurrent: 'Văn bản không còn hiệu lực',
  showNonCurrent: 'Xem kết quả từ văn bản đã hết hiệu lực', outside: 'Kết quả ngoài bộ lọc',
  matched: 'Khớp {n} khoản trong điều này:', expand: 'Xem đủ', collapse: 'Thu gọn',
  read: 'Đọc trong văn bản', copy: 'Sao chép trích dẫn', warningMore: 'và {n} cảnh báo khác', matchedWords: 'Từ trùng khớp',
}
export function searchMessages(language: Language): Record<keyof typeof en, string> {
  return language === 'en' ? en : vi
}
