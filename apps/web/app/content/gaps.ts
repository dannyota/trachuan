import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'
import { formatNumber } from './format'

type CategoryKey = 'content' | 'structure' | 'validity' | 'relations' | 'scope' | 'ocr' | 'provenance' | 'other'
export type GapCategory = { key: CategoryKey, name: string, sentence: string }
export type GapSummary = GapCategory & { cases: number, casesLabel: string }

const categories: Record<CategoryKey, Record<Language, { name: string, sentence: string }>> = {
  content: { vi: { name: 'Nội dung thiếu hoặc không đọc được', sentence: 'Một số nội dung bị thiếu hoặc không đọc được.' }, en: { name: 'Content missing or unreadable', sentence: 'Some content is missing or unreadable.' } },
  structure: { vi: { name: 'Cấu trúc chưa chắc chắn', sentence: 'Cấu trúc của một số văn bản chưa chắc chắn.' }, en: { name: 'Structure uncertain', sentence: 'The structure of some documents is uncertain.' } },
  validity: { vi: { name: 'Thông tin hiệu lực chưa đầy đủ', sentence: 'Thông tin hiệu lực của một số văn bản chưa đầy đủ.' }, en: { name: 'Validity incomplete', sentence: 'Validity information is incomplete for some documents.' } },
  relations: { vi: { name: 'Liên kết giữa các văn bản chưa đầy đủ', sentence: 'Một số liên kết giữa các văn bản chưa đầy đủ.' }, en: { name: 'Links between documents incomplete', sentence: 'Some links between documents are incomplete.' } },
  scope: { vi: { name: 'Thay đổi phạm vi nguồn', sentence: 'Một số văn bản đã bị gỡ, thay thế hoặc giữ lại từ bản trước.' }, en: { name: 'Source scope changes', sentence: 'Some documents were removed, replaced or retained from an earlier copy.' } },
  ocr: { vi: { name: 'Nhận dạng ký tự đang được xem xét', sentence: 'Một số kết quả nhận dạng ký tự đang được xem xét.' }, en: { name: 'OCR under review', sentence: 'Some character recognition results are under review.' } },
  provenance: { vi: { name: 'Thông tin nguồn chưa đầy đủ', sentence: 'Thông tin về nguồn của một số văn bản chưa đầy đủ.' }, en: { name: 'Source information incomplete', sentence: 'Source information is incomplete for some documents.' } },
  other: { vi: { name: 'Khác', sentence: 'Có những trường hợp khác cần xem xét.' }, en: { name: 'Other', sentence: 'Other cases need review.' } },
}

const kinds: Record<Exclude<CategoryKey, 'other'>, readonly string[]> = {
  content: ['chunk_omission', 'empty_text', 'file_format_unsupported', 'file_too_large', 'image_content', 'missing_attachment', 'no_chunks', 'no_text', 'no_text_source', 'no_tree', 'source_unavailable', 'table_text', 'text_limit', 'text_source_ambiguous', 'tree_too_large', 'not_found', 'response_too_large'],
  structure: ['ambiguous_tree_text', 'article_number_out_of_range', 'article_sequence', 'structure_uncertain', 'text_number_mismatch', 'tree_headings_only', 'tree_headings_partial', 'unknown_node_kind', 'invalid_ancestry', 'seed_article_missing'],
  validity: ['consolidated_status_unordered', 'consolidated_status_unresolved', 'invalid_date', 'invalid_validity_interval', 'missing_effective_date', 'missing_commencement', 'multiple_commencement_sentences', 'multiple_effect_articles', 'partial_status_unexplained', 'unknown_status', 'validity_conflict', 'correction_evidence_unavailable'],
  relations: ['named_document_ambiguous', 'named_document_missing', 'unknown_relation_code', 'unknown_target', 'unresolved_provision_relation', 'override_target_missing'],
  scope: ['scope_removed', 'source_replaced', 'withdrawn', 'source_withdrawn_retained'],
  ocr: ['needs_ocr', 'ocr_flagged', 'ocr_label_flagged', 'ocr_rejected'],
  provenance: ['source_provenance_missing', 'detail_id_mismatch', 'invalid_source_data', 'listing_unstable', 'source_identity_ambiguous'],
}

export function gapCategory(kind: string | undefined, lang: Language): GapCategory {
  const key = (Object.keys(kinds) as Array<keyof typeof kinds>).find(candidate => kinds[candidate].includes(kind || '') || (candidate === 'relations' && !!kind?.startsWith('relation_'))) || 'other'
  return { key, ...categories[key][lang] }
}

export function gapSummary(coverage: Pick<components['schemas']['CorpusStatus'], 'gaps'>, lang: Language): GapSummary[] {
  const grouped = new Map<CategoryKey, GapSummary>()
  for (const [kind, count] of Object.entries(coverage.gaps || {})) {
    if (count <= 0) continue
    const category = gapCategory(kind, lang)
    const cases = (grouped.get(category.key)?.cases || 0) + count
    grouped.set(category.key, { ...category, cases, casesLabel: `${formatNumber(cases, lang)} ${lang === 'vi' ? 'trường hợp' : 'cases'}` })
  }
  return [...grouped.values()].sort((a, b) => b.cases - a.cases || a.key.localeCompare(b.key))
}
