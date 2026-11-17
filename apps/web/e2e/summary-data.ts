import type { components } from '@trachuan/schema/gen/openapi'

export const forbiddenMarkers = ['PRIVATE_PROVISION_MARKER', 'PRIVATE_ANNEX_MARKER', 'EXCLUDED_OPERATIVE_HEADING', 'PHAN_HEADING_MARKER', 'MUC_HEADING_MARKER', 'UNKNOWN_KIND_HEADING', 'ARTICLE_SEQUENCE_HEADING', 'TREE_PARTIAL_HEADING', 'LONG_HEADING_MARKER', 'OCR_HEADING_MARKER', 'UNAPPROVED_HEADING_MARKER']

export const publicSummary: components['schemas']['PublicSummary'] = {
  release: '20990101_1', projection: 1, status_date: '2099-01-01', index: true,
  document: {
    key: 'vn:vbpl:1', number: '1/2099/QH', title: 'Luật mẫu </script><script>alert(1)</script>', type: 'Luật', issuer: 'Quốc hội',
    issued: '2098-12-01', effective_from: '2099-01-01', effective_to: null,
    status: { class: 'in_force', current: true, basis: 'source_status', status_basis: 'source_status', unresolved: false },
    gap_kinds: [], has_text: true, text_complete: true, source_url: 'https://example.invalid/document',
  },
  relations: [{ kind: 'amends_supplements', direction: 'incoming', key: 'vn:vbpl:2', number: '2/2099/QH', title: 'Văn bản liên quan' }, { kind: 'cites', direction: 'outgoing', number: '3/2099/QH' }],
  relations_total: 2, nodes: [{ code: '1', name: 'Chủ đề mẫu' }],
  outline: [{ kind: 'phan', label: 'Phần I', anchor: 'phan-i' }, { kind: 'chuong', label: 'Chương I', anchor: 'chuong-i', heading: 'Chương được duyệt' }, { kind: 'muc', label: 'Mục 1', anchor: 'muc-1' }, { kind: 'dieu', label: 'Điều 1', anchor: 'dieu-1', heading: 'Điều được duyệt' }],
}
