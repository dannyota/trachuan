import type { Language } from './index'

const en = {
  law: 'Legal documents', frameworks: 'Control frameworks', search: 'Search', filter: 'Framework', all: 'All frameworks', more: 'Show more', next: 'Show more',
  free: 'Free to read', licensed: 'Access required', readable: 'You can read this', unreadable: 'You do not have access',
  accessUnavailable: 'Access could not be checked right now; this shows the free part.',
  account: 'Access is granted to the verified email of an account.',
  withheld: "The publisher's text needs access to read. This account does not have it.",
  counts: (controls: number, resolved: number, unresolved: number) => `${controls} controls · ${resolved} resolved mappings · ${unresolved} unresolved mappings`,
  mappings: 'Mappings', children: 'Contents', gaps: 'Data limits', sources: 'Sources', complete: 'Complete', incomplete: 'Incomplete', lookup: 'Citation match',
  unit_too_large: 'Part of this item is too large to show.', invalid_item: 'This item could not be read.', reload: 'The data was updated; the page starts again from the top.',
  relation: { equivalent: 'equivalent to', subset: 'subset of', superset: 'superset of', intersects: 'intersects', related: 'related to' },
  gap: { mapping_unresolved: 'Mapping target not resolved', mapping_dropped: 'Mapping left out of the release', title_missing: 'No Tra Chuẩn title yet', ocr_blocked: 'Character recognition did not pass', ocr_flagged: 'Character recognition under review', text_incomplete: 'Text incomplete', chunk_omission: 'Part of the text is not searchable', embedding_input_too_long: 'Part of the text is too long to search', embedding_input_bytes_exceeded: 'Part of the text is too long to search' },
}
const vi: typeof en = {
  law: 'Văn bản pháp luật', frameworks: 'Khung kiểm soát', search: 'Tra cứu', filter: 'Khung kiểm soát', all: 'Tất cả khung kiểm soát', more: 'Xem thêm', next: 'Xem tiếp',
  free: 'Đọc tự do', licensed: 'Cần cấp quyền', readable: 'Bạn đọc được', unreadable: 'Bạn chưa được cấp quyền',
  accessUnavailable: 'Tạm thời chưa kiểm tra được quyền đọc; trang đang hiện phần đọc tự do.',
  account: 'Quyền đọc được cấp theo địa chỉ email đã xác minh của tài khoản.',
  withheld: 'Nội dung của nhà xuất bản cần được cấp quyền mới đọc được. Tài khoản này chưa được cấp quyền.',
  counts: (controls, resolved, unresolved) => `${controls} mục kiểm soát · ${resolved} liên kết đã xác định đích · ${unresolved} liên kết chưa xác định đích`,
  mappings: 'Liên kết', children: 'Mục lục', gaps: 'Giới hạn dữ liệu', sources: 'Nguồn', complete: 'Đầy đủ', incomplete: 'Chưa đầy đủ', lookup: 'Tra theo số hiệu',
  unit_too_large: 'Một phần của mục này quá lớn để hiển thị.', invalid_item: 'Không đọc được mục này.', reload: 'Dữ liệu đã được cập nhật; trang hiện lại từ đầu.',
  relation: { equivalent: 'tương đương', subset: 'là một phần của', superset: 'bao gồm', intersects: 'giao với', related: 'liên quan' },
  gap: { mapping_unresolved: 'Liên kết chưa xác định đích', mapping_dropped: 'Liên kết không đưa vào bản phát hành', title_missing: 'Chưa có tên do Tra Chuẩn đặt', ocr_blocked: 'Nhận dạng ký tự chưa đạt', ocr_flagged: 'Nhận dạng ký tự đang được xem xét', text_incomplete: 'Nội dung chưa đầy đủ', chunk_omission: 'Một phần nội dung không được đưa vào tìm kiếm', embedding_input_too_long: 'Một phần nội dung quá dài để đưa vào tìm kiếm', embedding_input_bytes_exceeded: 'Một phần nội dung quá dài để đưa vào tìm kiếm' },
}
export function frameworkMessages(language: Language): typeof en { return language === 'en' ? en : vi }
