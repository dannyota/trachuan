import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'

type Warning = components['schemas']['Warning']
type Document = Pick<components['schemas']['Document'], 'type' | 'number'>

function anchorLabels(anchor: string | undefined): string[] | null {
  if (!anchor) return null
  const article = /^dieu-([0-9]+[a-zđ0-9]*)(?:\/khoan-([0-9]+[a-zđ0-9]*))?(?:\/diem-([a-zđ][a-zđ0-9]*))?$/i.exec(anchor)
  if (article) return [`Điều ${article[1]}`, ...(article[2] ? [`Khoản ${article[2]}`] : []), ...(article[3] ? [`Điểm ${article[3]}`] : [])]
  const names: Record<string, string> = { phan: 'Phần', chuong: 'Chương', muc: 'Mục' }
  const order: Record<string, number> = { phan: 0, chuong: 1, muc: 2 }
  const parts = anchor.split('/')
  let previous = -1
  const labels: string[] = []
  for (const part of parts) {
    const match = /^(phan|chuong|muc)-([a-zđ0-9]+)$/i.exec(part)
    if (!match) return null
    const kind = match[1]!.toLowerCase()
    if (order[kind]! <= previous) return null
    previous = order[kind]!
    const token = match[2]!
    labels.push(`${names[kind]} ${/^[ivxlcdm]+$/i.test(token) ? token.toUpperCase() : token}`)
  }
  return labels.length ? labels : null
}

export function citationFromAnchor(anchor: string | undefined): string | null {
  return anchorLabels(anchor)?.join(', ') || null
}

export function documentCitation(anchor: string | undefined, doc: Document): string | null {
  const labels = anchorLabels(anchor)
  return labels ? [labels.reverse().join(' '), doc.type, doc.number].filter(Boolean).join(' ') : null
}

const verbs: Record<string, Record<Language, string>> = {
  repeals: { vi: 'bãi bỏ', en: 'repeals' },
  replaces: { vi: 'thay thế', en: 'replaces' },
  suspends: { vi: 'ngưng hiệu lực', en: 'suspends' },
  amends_supplements: { vi: 'sửa đổi, bổ sung', en: 'amends or supplements' },
}

export function warningSentence(warning: Warning, lang: Language): string {
  const target = (warning.exact ? warning.citation : citationFromAnchor(warning.named)) || (lang === 'vi' ? 'một phần của văn bản' : 'part of this document')
  const actor = warning.number || (lang === 'vi' ? 'Văn bản liên quan' : 'A related document')
  const verb = warning.kind && Object.hasOwn(verbs, warning.kind) ? verbs[warning.kind]![lang] : (lang === 'vi' ? 'liên quan đến' : 'relates to')
  const missing = warning.exact ? '' : (lang === 'vi' ? ' (khoản hoặc điểm này chưa có trong văn bản đã tải)' : ' (this clause or point is not in the loaded text)')
  const reference = warning.basis === 'vbpl_reference' ? (lang === 'vi' ? ' (theo liên kết của VBPL)' : ' (per a VBPL link)') : ''
  return `${actor} ${verb} ${target}${missing}${reference}.`
}
