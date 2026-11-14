import type { Language } from './index'

export const en = {
  close: 'Close', backToTop: 'Back to top', back: '← Back to results', issued: 'Issued', effectiveFrom: 'Effective from', effectiveTo: 'Expires', release: 'Release',
  source: 'Original at vbpl.vn ↗', copyDocument: 'Copy document citation', contents: 'Contents', targetArticle: 'Article you are viewing', targetClause: 'Clause you are viewing', targetPoint: 'Point you are viewing', copy: 'Copy:',
  relations: 'Relations to other documents', topics: 'Topics', technical: 'Technical details', warnings: 'Warnings', loading: 'Loading…',
  retry: 'Retry', moreRelations: 'Load more relations', legalBases: 'Legal bases', sourceUrl: 'Source', gaps: 'Gaps',
}
export const vi: Record<keyof typeof en, string> = {
  close: 'Đóng', backToTop: 'Lên đầu trang', back: '← Quay lại kết quả', issued: 'Ban hành', effectiveFrom: 'Hiệu lực từ', effectiveTo: 'Hết hiệu lực', release: 'Bản phát hành',
  source: 'Bản gốc trên vbpl.vn ↗', copyDocument: 'Sao chép trích dẫn văn bản', contents: 'Mục lục', targetArticle: 'Điều bạn đang xem', targetClause: 'Khoản bạn đang xem', targetPoint: 'Điểm bạn đang xem', copy: 'Sao chép:',
  relations: 'Quan hệ với văn bản khác', topics: 'Chủ đề', technical: 'Chi tiết kỹ thuật', warnings: 'Cảnh báo', loading: 'Đang tải…',
  retry: 'Thử lại', moreRelations: 'Tải thêm quan hệ', legalBases: 'Căn cứ ban hành', sourceUrl: 'Nguồn', gaps: 'Giới hạn dữ liệu',
}
export function readerMessages(language: Language): Record<keyof typeof en, string> {
  return language === 'en' ? en : vi
}
