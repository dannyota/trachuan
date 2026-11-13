import { expect, it } from 'vitest'
import * as content from '../../app/content'

it('labels status and its basis', () => {
  expect(content.statusLabel({ current: true, class: 'in_force' }, 'vi')).toBe('Còn hiệu lực')
  expect(content.statusLabel({ current: false, class: 'partial' }, 'en')).toBe('Partly in force')
  expect(content.statusLabel(undefined, 'vi')).toBe('Chưa rõ hiệu lực')
  expect(content.statusBasisLabel({ current: true, status_basis: 'source_status' }, 'https://vbpl.vn/x', 'vi')).toBe('theo vbpl.vn')
  expect(content.statusBasisLabel({ current: true, status_basis: 'derived_from_text' }, undefined, 'vi')).toBe('suy ra từ nội dung văn bản')
  expect(content.statusBasisLabel(undefined, undefined, 'en')).toBeNull()
})
it('formats the document byline without missing parts', () => {
  expect(content.hitByline({ type: 'Thông tư', issuer: 'Ngân hàng Nhà nước Việt Nam', issued: '2024-10-31' }, 'vi')).toBe('Thông tư · Ngân hàng Nhà nước Việt Nam · 31/10/2024')
  expect(content.hitByline({ issuer: 'Cơ quan' }, 'en')).toBe('Cơ quan')
})
it('selects descendant body fragments only, in order', () => {
  const fragment = (anchor: string, role: string, document_key = 'doc') => ({ identity: { document_key, anchor }, role, start: 0, end: 1, text_start: 0, text_end: 1, complete: true })
  const hit = { complete: true, doc: 'doc', handle: 'doc#dieu-5', fragments: [fragment('dieu-5', 'body'), fragment('dieu-5/khoan-2a', 'body'), fragment('dieu-5/khoan-3', 'lead'), fragment('dieu-50/khoan-1', 'body'), fragment('dieu-5/khoan-4', 'body', 'other'), fragment('dieu-5/khoan-1', 'body')] }
  expect(content.matchedMembers(hit)).toEqual(['Điều 5, Khoản 2a', 'Điều 5, Khoản 1'])
})
it('groups pointers and counts undisplayed entries', () => {
  const a = { dir: 'in', kind: 'repeals', number: '1', in_release: true }
  const b = { dir: 'out', kind: 'cites', number: '2', in_release: false }
  expect(content.relationGroups([a, b, a], { 'in:repeals': 4, 'out:cites': 1, 'in:replaces': 2 }, 'vi')).toEqual([
    { label: 'Bị bãi bỏ bởi', dir: 'in', kind: 'repeals', items: [a, a], more: 2 },
    { label: 'Dẫn chiếu', dir: 'out', kind: 'cites', items: [b], more: 0 },
  ])
})
it('selects outward legal bases and gives warning advice', () => {
  const a = { dir: 'out', kind: 'legal_basis', in_release: true, evidence_complete: true }
  const b = { ...a, dir: 'in' }
  expect(content.legalBases([a, b])).toEqual([a])
  expect(content.warningAdvice('amends_supplements', 'vi')).toBe('Đối chiếu văn bản sửa đổi trước khi dẫn chiếu.')
  expect(content.warningAdvice('cites', 'en')).toBeNull()
})
it('highlights maximal syllable runs in the original text', () => {
  const text = '😀 ĐỘ DÀI tối thiểu 08 ký tự; và mã khác.'
  const spans = content.highlightSpans(text, 'độ dài tối thiểu 08 ký tự')
  expect(spans.map(span => text.slice(span.start, span.end))).toEqual(['ĐỘ DÀI tối thiểu 08 ký tự'])
})
it('highlights single digit syllables but not single words', () => {
  const text = 'Có 08 mục và 50/2024/TT-NHNN.'
  expect(content.highlightSpans(text, '08 có').map(span => text.slice(span.start, span.end))).toEqual(['08'])
  expect(content.highlightSpans(text, 'và')).toEqual([])
  expect(content.highlightSpans(text, '2024').map(span => text.slice(span.start, span.end))).toEqual(['2024'])
})
it('normalizes and folds case while preserving diacritics and offsets', () => {
  const text = '😀 MÃ văn bản, ma văn bản; VĂN BẢN!'
  const spans = content.highlightSpans(text, 'mã văn bản')
  expect(spans.map(span => text.slice(span.start, span.end))).toEqual(['MÃ văn bản', 'văn bản', 'VĂN BẢN'])
  expect(content.highlightSpans('mã', 'ma')).toEqual([])
})
it('splits punctuation and chooses non-overlapping maximal runs', () => {
  const text = 'Điều, khoản-điểm; Điều khoản 2.'
  const spans = content.highlightSpans(text, 'điều khoản điểm')
  expect(spans.map(span => text.slice(span.start, span.end))).toEqual(['Điều, khoản-điểm', 'Điều khoản'])
  expect(content.highlightSpans('khác', 'điều khoản')).toEqual([])
})
it('prefers an available fragment identity citation', () => {
  const fragment = { identity: { document_key: 'doc', anchor: 'dieu-5/khoan-2', citation: 'Điều 5, Khoản 2 đặc biệt' }, role: 'body', start: 0, end: 1, text_start: 0, text_end: 1, complete: true }
  expect(content.matchedMembers({ complete: true, handle: 'doc#dieu-5', fragments: [fragment] })).toEqual(['Điều 5, Khoản 2 đặc biệt'])
})
it('uses case folding for alternate letter forms', () => {
  const text = 'Straße Σίσυφος ı İ'
  expect(content.highlightSpans(text, 'STRASSE ΣΊΣΥΦΟΣ').map(span => text.slice(span.start, span.end))).toEqual(['Straße Σίσυφος'])
  expect(content.highlightSpans('ı 1', 'i 1')).toEqual([{ start: 2, end: 3 }])
})
