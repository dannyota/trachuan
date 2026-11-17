import type { Language } from './index'

const en = {
  read: 'Read the full text', incomplete: 'The document text is incomplete.', missing: 'Tra Chuẩn does not have the text of this document yet.',
  unresolved: 'Validity unresolved: the sources disagree', statusDate: 'Status recorded for', relationsTotal: 'Total relations',
}
const vi: Record<keyof typeof en, string> = {
  read: 'Đọc toàn văn', incomplete: 'Nội dung văn bản chưa đầy đủ.', missing: 'Tra Chuẩn chưa có nội dung văn bản này.',
  unresolved: 'Hiệu lực chưa xác định: các nguồn mâu thuẫn', statusDate: 'Trạng thái ghi nhận ngày', relationsTotal: 'Tổng số quan hệ',
}
export function summaryMessages(language: Language) { return language === 'en' ? en : vi }
