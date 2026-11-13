import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'

type Warning = components['schemas']['Warning']
type Document = Pick<components['schemas']['Document'], 'type' | 'number'>

export function citationFromAnchor(anchor: string | undefined): string | null {
  if (!anchor) return null
  const article = /^dieu-([0-9]+[a-zđ0-9]*)(?:\/khoan-([0-9]+[a-zđ0-9]*)(?:\/diem-([a-zđ][a-zđ0-9]*))?)?$/i.exec(anchor)
  if (article) return [article[3] && `Điểm ${article[3]}`, article[2] && `Khoản ${article[2]}`, `Điều ${article[1]}`].filter(Boolean).join(' ')
  const section = /^(chuong|muc|phan)-([a-zđ0-9]+)$/i.exec(anchor)
  if (!section) return null
  const names: Record<string, string> = { chuong: 'Chương', muc: 'Mục', phan: 'Phần' }
  return `${names[section[1]!.toLowerCase()]} ${section[2]}`
}

export function documentCitation(anchor: string | undefined, doc: Document): string | null {
  const citation = citationFromAnchor(anchor)
  return citation ? [citation, doc.type, doc.number].filter(Boolean).join(' ') : null
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
  return `${actor} ${verb} ${target}.`
}
