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
  ['suspends', 'Bị ngưng hiệu lực bởi', 'Ngưng hiệu lực', 'Suspended by', 'Suspends'],
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
  expect(citationFromAnchor('dieu-1/khoan-9/diem-a')).toBe('Điều 1, Khoản 9, Điểm a')
  expect(citationFromAnchor('dieu-5/khoan-2a')).toBe('Điều 5, Khoản 2a')
  expect(citationFromAnchor('dieu-3')).toBe('Điều 3')
  expect(citationFromAnchor('chuong-I')).toBe('Chương I')
  expect(citationFromAnchor('muc-2')).toBe('Mục 2')
  expect(citationFromAnchor('phan-I')).toBe('Phần I')
  expect(citationFromAnchor('dieu-1/diem-a')).toBe('Điều 1, Điểm a')
  expect(documentCitation('dieu-11/khoan-1', { type: 'Thông tư', number: '50/2024/TT-NHNN' })).toBe('Khoản 1 Điều 11 Thông tư 50/2024/TT-NHNN')
})
it('uses exact citations or named paths in warnings', () => {
  const warning = { in_release: true, basis: 'text', target: 'key#dieu-11', exact: true, citation: 'Điều 11, Khoản 1', number: '77/2025/TT-NHNN', kind: 'amends_supplements' }
  expect(warningSentence(warning, 'vi')).toBe('77/2025/TT-NHNN sửa đổi, bổ sung Điều 11, Khoản 1.')
  expect(warningSentence({ ...warning, exact: false, named: 'dieu-11/khoan-2a' }, 'en')).toBe('77/2025/TT-NHNN amends or supplements Điều 11, Khoản 2a (this clause or point is not in the loaded text).')
  expect(warningSentence({ ...warning, exact: false, named: 'raw-anchor' }, 'vi')).toContain('một phần của văn bản')
})

it('groups gap rows as cases, including global rows', () => {
  const result = gapSummary({ gaps: { no_tree: 3842, empty_text: 2, invalid_date: 3, relation_missing: 1, mystery: 0 } }, 'vi')
  expect(result[0]).toMatchObject({ cases: 3844, casesLabel: '3.844 trường hợp', casesSentence: 'Một số nội dung bị thiếu hoặc không đọc được (3.844 trường hợp).' })
  expect(gapSummary({ gaps: { missing_effective_date: 120 } }, 'en')[0]!.casesSentence).toBe('Validity information is incomplete for some documents (120 cases).')
  expect(result.map(row => row.cases)).toEqual([3844, 3, 1])
  expect(gapCategory('relation_any', 'en').name).toBeTruthy()
  expect(gapCategory('something_new', 'vi').name).toBe('Khác')
})
it.each(['vi', 'en'] as const)('classifies and groups partial trees and retained baselines in %s', (lang) => {
  expect(gapCategory('tree_text_partial', lang).key).toBe('structure')
  expect(gapCategory('baseline_served', lang).key).toBe('scope')
  const result = gapSummary({ gaps: { tree_text_partial: 2, tree_headings_partial: 3, baseline_served: 4, scope_removed: 5 } }, lang)
  expect(result.map(({ key, cases }) => ({ key, cases }))).toEqual([
    { key: 'scope', cases: 9 },
    { key: 'structure', cases: 5 },
  ])
})
it.each([
  ['vi', 'Kết quả kiểm tra quan hệ chưa được đưa vào', 'Một số kết quả kiểm tra quan hệ giữa các văn bản chưa được đưa vào bản phát hành này.'],
  ['en', 'Relation check results not included', 'Some check results on relations between documents are not included in this release.'],
] as const)('classifies and groups knowledge_stale in %s', (lang, name, sentence) => {
  expect(gapCategory('knowledge_stale', lang)).toEqual({ key: 'knowledge', name, sentence })
  const result = gapSummary({ gaps: { knowledge_stale: 7 } }, lang)
  const label = lang === 'vi' ? '7 trường hợp' : '7 cases'
  expect(result).toEqual([{ key: 'knowledge', name, sentence, cases: 7, casesLabel: label, casesSentence: `${sentence.replace(/\.$/, '')} (${label}).` }])
})
it('uses the singular case label in English for a count of one', () => {
  expect(gapSummary({ gaps: { knowledge_stale: 1 } }, 'en')[0]).toMatchObject({ casesLabel: '1 case', casesSentence: 'Some check results on relations between documents are not included in this release (1 case).' })
  expect(gapSummary({ gaps: { knowledge_stale: 1 } }, 'vi')[0]!.casesLabel).toBe('1 trường hợp')
})
it.each([
  ['vi', 'Trang thông tin văn bản chưa đầy đủ', 'Nội dung văn bản không bị ảnh hưởng, nhưng trang thông tin của một số văn bản thiếu một phần hoặc không có vì dữ liệu vượt giới hạn hiển thị.', 'trường hợp'],
  ['en', 'Document information pages incomplete', 'Document text is not affected, but information pages for some documents are partial or unavailable because their data exceeds display limits.', 'cases'],
] as const)('classifies and groups summary and stale waiver gaps in %s', (lang, name, sentence, unit) => {
  expect(gapCategory('ocr_waiver_stale', lang).key).toBe('ocr')
  for (const kind of ['summary_field_capped', 'summary_outline_capped', 'summary_row_capped']) expect(gapCategory(kind, lang)).toEqual({ key: 'summary', name, sentence })
  const result = gapSummary({ gaps: { summary_field_capped: 1, summary_outline_capped: 2, summary_row_capped: 3 }, features: { public_summaries: true } }, lang)
  const label = `6 ${unit}`
  expect(result).toEqual([{ key: 'summary', name, sentence, cases: 6, casesLabel: label, casesSentence: `${sentence.replace(/\.$/, '')} (${label}).` }])
})
it('leaves out the summary category unless public summaries are on', () => {
  const gaps = { summary_row_capped: 3, no_tree: 1 }
  for (const status of [{ gaps }, { gaps, features: { public_summaries: false } }]) expect(gapSummary(status, 'en').map(row => row.key)).toEqual(['content'])
  expect(gapSummary({ gaps, features: { public_summaries: true } }, 'en').map(row => row.key)).toEqual(['summary', 'content'])
})
it.each([
  ['vi', 'Nhận dạng ký tự cần xem xét', 'Phần nhận dạng ký tự của một số văn bản cần xem xét.'],
  ['en', 'OCR needs review', 'Character recognition for some documents needs review.'],
] as const)('pins the OCR category text in %s', (lang, name, sentence) => {
  expect(gapCategory('ocr_flagged', lang)).toEqual({ key: 'ocr', name, sentence })
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
    for (const kind of ['no_tree', 'article_sequence', 'invalid_date', 'relation_missing', 'withdrawn', 'ocr_flagged', 'source_provenance_missing', 'listing_only', 'other']) {
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
  const kinds = ['ambiguous_tree_text', 'article_sequence', 'chunk_omission', 'consolidated_status_unordered', 'consolidated_status_unresolved', 'empty_text', 'file_format_unsupported', 'image_content', 'invalid_date', 'invalid_validity_interval', 'listing_only', 'missing_attachment', 'missing_effective_date', 'named_document_missing', 'needs_ocr', 'no_chunks', 'no_text_source', 'no_tree', 'ocr_flagged', 'ocr_label_flagged', 'ocr_rejected', 'partial_status_unexplained', 'relation_clause_unresolved', 'scope_removed', 'seed_article_missing', 'source_provenance_missing', 'source_replaced', 'source_unavailable', 'structure_uncertain', 'table_text', 'text_number_mismatch', 'text_source_ambiguous', 'unknown_relation_code', 'unknown_status', 'unknown_target', 'tree_headings_only', 'tree_headings_partial', 'withdrawn', 'source_withdrawn_retained']
  for (const kind of kinds) expect(gapCategory(kind, 'vi').key, kind).not.toBe('other')
  expect(gapCategory('listing_only', 'vi')).toMatchObject({ key: 'search', name: 'Không đưa vào tìm kiếm' })
  expect(gapCategory('listing_only', 'en').sentence).toBe('Some content stays readable but is not fully searchable.')
  expect(gapCategory('listing_only', 'vi').sentence).toBe('Một số nội dung vẫn đọc được nhưng không được đưa vào tìm kiếm đầy đủ.')
  for (const kind of ['embedding_input_too_long', 'embedding_input_bytes_exceeded']) expect(gapCategory(kind, 'en').key).toBe('search')
})

it('renders nested sections and inserted point tokens', () => {
  expect(citationFromAnchor('dieu-1a/diem-a1')).toBe('Điều 1a, Điểm a1')
  expect(citationFromAnchor('dieu-1/khoan-2a/diem-đ')).toBe('Điều 1, Khoản 2a, Điểm đ')
  expect(citationFromAnchor('chuong-i/muc-2')).toBe('Chương I, Mục 2')
  expect(citationFromAnchor('phan-i/chuong-ii/muc-1')).toBe('Phần I, Chương II, Mục 1')
})
it('adds warning notes and preserves exact citations', () => {
  const warning = { in_release: true, basis: 'vbpl_reference', target: 'key#dieu-8', exact: false, named: 'dieu-8/khoan-9/diem-a', citation: 'Điều 8', number: '77', kind: 'repeals' }
  expect(warningSentence(warning, 'vi')).toBe('77 bãi bỏ Điều 8, Khoản 9, Điểm a (khoản hoặc điểm này chưa có trong văn bản đã tải) (theo liên kết của VBPL).')
  expect(warningSentence({ ...warning, exact: true }, 'en')).toBe('77 repeals Điều 8 (per a VBPL link).')
})
it('rejects a non-existent date before timestamp conversion', () => {
  expect(formatDateTime('2026-02-30T14:31:00Z', 'vi')).toBe('2026-02-30T14:31:00Z')
})
it('describes the additional gaps and topic assignment accurately', () => {
  expect(gapCategory('relations_omitted', 'en').sentence).toContain('omitted')
  expect(gapCategory('historical_document_dates', 'en').sentence).toContain('document dates')
  expect(gapCategory('eval_floor_failed', 'en').sentence).toContain('evaluation')
  expect(gapCategory('eval_rejection_failed', 'en').sentence).toContain('rejection')
  for (const kind of ['seed_article_missing', 'override_target_missing']) {
    expect(gapCategory(kind, 'en').sentence).toContain('Topics may be missing for some provisions')
  }
})

it.each([
  ['vi', true, 'Nội dung: đầy đủ, từ tệp PDF của vbpl.vn'],
  ['vi', false, 'Nội dung: có thể chưa đầy đủ, từ tệp PDF của vbpl.vn'],
  ['en', true, 'Content: complete, from the PDF at vbpl.vn'],
  ['en', false, 'Content: may be incomplete, from the PDF at vbpl.vn'],
] as const)('labels PDF text in %s with completeness %s', (lang, complete, expected) => {
  expect(textSourceLabel('pdf_text', complete, 'https://vbpl.vn/doc', lang)).toBe(expected)
})
