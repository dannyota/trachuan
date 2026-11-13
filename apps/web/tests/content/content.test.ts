import { describe, expect, it } from 'vitest'
import { formatNumber, formatDate, formatDateTime, formatRelease, relationLabel, relationCountLabel, citationFromAnchor, documentCitation, warningSentence, gapCategory, gapSummary, textSourceLabel, statusAsOf, pinnedNotice } from '../../app/content'

it('formats numbers, civil dates, instants and releases', () => {
  expect(formatNumber(305906, 'vi')).toBe('305.906')
  expect(formatNumber(3842, 'en')).toBe('3,842')
  expect(formatDate('2026-11-12', 'vi')).toBe('12/11/2026')
  expect(formatDate('2026-11-12', 'en')).toBe('12 Nov 2026')
  expect(formatDate('2026-02-30', 'vi')).toBe('2026-02-30')
  expect(formatDateTime('2026-11-13T14:31:00Z', 'vi')).toBe('13/11/2026 21:31')
  expect(formatDateTime('2026-11-13T14:31:00Z', 'en')).toBe('13 Nov 2026, 21:31')
  expect(formatRelease('20261112_2', 'vi')).toBe('12/11/2026 (bản 2)')
  expect(formatRelease('20261112_2', 'en')).toBe('12 Nov 2026 (build 2)')
  expect(formatRelease('20261112', 'en')).toBe('12 Nov 2026')
  expect(formatRelease('20261312_2', 'en')).toBe('20261312_2')
})

const relations = [
  ['repeals', 'Bị bãi bỏ bởi', 'Bãi bỏ', 'Repealed by', 'Repeals'],
  ['replaces', 'Bị thay thế bởi', 'Thay thế', 'Replaced by', 'Replaces'],
  ['suspends', 'Bị đình chỉ thi hành bởi', 'Đình chỉ thi hành', 'Suspended by', 'Suspends'],
  ['amends_supplements', 'Được sửa đổi, bổ sung bởi', 'Sửa đổi, bổ sung', 'Amended by', 'Amends'],
  ['consolidates', 'Được hợp nhất trong', 'Hợp nhất', 'Consolidated in', 'Consolidates'],
  ['implements', 'Được quy định chi tiết, hướng dẫn bởi', 'Quy định chi tiết, hướng dẫn', 'Implemented by', 'Implements'],
  ['guided_by', 'Hướng dẫn', 'Được hướng dẫn bởi', 'Guides', 'Guided by'],
  ['legal_basis', 'Là căn cứ ban hành của', 'Căn cứ ban hành', 'Legal basis for', 'Based on'],
  ['cites', 'Được dẫn chiếu bởi', 'Dẫn chiếu', 'Cited by', 'Cites'],
  ['references', 'Văn bản liên quan', 'Văn bản liên quan', 'Related', 'Related'],
  ['unknown', 'Quan hệ khác', 'Quan hệ khác', 'Other relation', 'Other relation'],
] as const
it.each(relations)('labels %s in both directions and languages', (kind, viIn, viOut, enIn, enOut) => {
  expect(relationLabel(kind, 'in', 'vi')).toBe(viIn)
  expect(relationLabel(kind, 'out', 'vi')).toBe(viOut)
  expect(relationLabel(kind, 'in', 'en')).toBe(enIn)
  expect(relationLabel(kind, 'out', 'en')).toBe(enOut)
  expect(relationCountLabel(`in:${kind}`, 'en')).toBe(enIn)
})
it('does not expose unknown relation codes', () => {
  expect(relationCountLabel('other:raw', 'vi')).toBe('Quan hệ khác')
})

it('renders anchors in citation order without exposing malformed anchors', () => {
  expect(citationFromAnchor('dieu-1/khoan-9/diem-a')).toBe('Điểm a Khoản 9 Điều 1')
  expect(citationFromAnchor('dieu-5/khoan-2a')).toBe('Khoản 2a Điều 5')
  expect(citationFromAnchor('dieu-3')).toBe('Điều 3')
  expect(citationFromAnchor('chuong-I')).toBe('Chương I')
  expect(citationFromAnchor('muc-2')).toBe('Mục 2')
  expect(citationFromAnchor('phan-I')).toBe('Phần I')
  expect(citationFromAnchor('dieu-1/diem-a')).toBeNull()
  expect(documentCitation('dieu-11/khoan-1', { type: 'Thông tư', number: '50/2024/TT-NHNN' })).toBe('Khoản 1 Điều 11 Thông tư 50/2024/TT-NHNN')
})
it('uses exact citations or named paths in warnings', () => {
  const warning = { in_release: true, basis: 'text', target: 'key#dieu-11', exact: true, citation: 'Khoản 1 Điều 11', number: '77/2025/TT-NHNN', kind: 'amends_supplements' }
  expect(warningSentence(warning, 'vi')).toBe('77/2025/TT-NHNN sửa đổi, bổ sung Khoản 1 Điều 11.')
  expect(warningSentence({ ...warning, exact: false, named: 'dieu-11/khoan-2a' }, 'en')).toBe('77/2025/TT-NHNN amends or supplements Khoản 2a Điều 11.')
  expect(warningSentence({ ...warning, exact: false, named: 'raw-anchor' }, 'vi')).toContain('một phần của văn bản')
})

it('groups gap rows as cases, including global rows', () => {
  const result = gapSummary({ gaps: { no_tree: 3842, empty_text: 2, invalid_date: 3, relation_missing: 1, mystery: 0 } }, 'vi')
  expect(result[0]).toMatchObject({ cases: 3844, casesLabel: '3.844 trường hợp' })
  expect(result.map(row => row.cases)).toEqual([3844, 3, 1])
  expect(gapCategory('relation_any', 'en').name).toBeTruthy()
  expect(gapCategory('something_new', 'vi').name).toBe('Khác')
})
it('labels provenance and source withdrawal', () => {
  expect(textSourceLabel('source_tree', true, 'https://vbpl.vn/example', 'vi')).toBe('Nội dung: đầy đủ, từ cấu trúc văn bản của vbpl.vn')
  expect(textSourceLabel('html_transcription', false, 'https://chinhphu.vn/x', 'vi')).toContain('có thể chưa đầy đủ, từ bản HTML của chinhphu.vn')
  expect(textSourceLabel('ocr', true, undefined, 'vi')).toContain('từ bản quét (nhận dạng ký tự) đã kiểm tra')
  expect(textSourceLabel('none', false, undefined, 'vi')).toBe('Chưa có nội dung')
  expect(statusAsOf('2026-11-13', 'https://vbpl.vn/a', 'vi')).toBe('Trạng thái tính đến 13/11/2026, theo vbpl.vn')
  expect(pinnedNotice('2026-11-13', 'vi')).toBe('Nguồn đã gỡ văn bản này từ 13/11/2026; nội dung lấy từ bản đã kiểm tra trước đó.')
  expect(pinnedNotice(undefined, 'en')).toBeNull()
})

describe('language counterparts', () => {
  it('returns English text for every relation, gap category and source state', () => {
    for (const [kind] of relations) for (const dir of ['in', 'out']) {
      expect(relationLabel(kind, dir, 'vi')).toBeTruthy()
      expect(relationLabel(kind, dir, 'en')).toBeTruthy()
    }
    for (const kind of ['no_tree', 'article_sequence', 'invalid_date', 'relation_missing', 'withdrawn', 'ocr_flagged', 'source_provenance_missing', 'other']) {
      expect(gapCategory(kind, 'vi').sentence).toBeTruthy()
      expect(gapCategory(kind, 'en').sentence).toBeTruthy()
    }
    for (const source of ['source_tree', 'html_transcription', 'ocr', 'none']) {
      expect(textSourceLabel(source, true, 'https://vbpl.vn', 'vi')).toBeTruthy()
      expect(textSourceLabel(source, true, 'https://vbpl.vn', 'en')).toBeTruthy()
    }
  })
})

it('classifies corpus gap kinds instead of displaying internal names', () => {
  const kinds = ['ambiguous_tree_text', 'article_sequence', 'chunk_omission', 'consolidated_status_unordered', 'consolidated_status_unresolved', 'empty_text', 'file_format_unsupported', 'image_content', 'invalid_date', 'invalid_validity_interval', 'missing_attachment', 'missing_effective_date', 'named_document_missing', 'needs_ocr', 'no_chunks', 'no_text_source', 'no_tree', 'ocr_flagged', 'ocr_label_flagged', 'ocr_rejected', 'partial_status_unexplained', 'relation_clause_unresolved', 'scope_removed', 'seed_article_missing', 'source_provenance_missing', 'source_replaced', 'source_unavailable', 'structure_uncertain', 'table_text', 'text_number_mismatch', 'text_source_ambiguous', 'unknown_relation_code', 'unknown_status', 'unknown_target', 'tree_headings_only', 'tree_headings_partial', 'withdrawn', 'source_withdrawn_retained']
  for (const kind of kinds) expect(gapCategory(kind, 'vi').key, kind).not.toBe('other')
})
