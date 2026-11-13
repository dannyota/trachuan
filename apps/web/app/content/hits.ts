import type { components } from '@trachuan/schema/gen/openapi'
import type { Language } from '../../i18n'
import { citationFromAnchor } from './citations'
import { formatDate } from './format'

type Document = components['schemas']['Document']
type Hit = components['schemas']['Hit']

export function hitByline(doc: Pick<Document, 'type' | 'issuer' | 'issued'>, lang: Language): string {
  return [doc.type, doc.issuer, doc.issued ? formatDate(doc.issued, lang) : undefined].filter(Boolean).join(' · ')
}

export function matchedMembers(hit: Hit): string[] {
  const separator = hit.handle?.indexOf('#') ?? -1
  if (separator < 0 || !hit.handle) return []
  const doc = hit.handle.slice(0, separator), anchor = hit.handle.slice(separator + 1)
  if (!doc || !/^dieu-[0-9]+[a-zđ0-9]*$/i.test(anchor)) return []
  const seen = new Set<string>()
  return (hit.fragments || []).flatMap((fragment) => {
    const identity = fragment.identity
    if (fragment.role !== 'body' || identity?.document_key !== doc || !identity.anchor?.startsWith(`${anchor}/`)) return []
    const member = identity.anchor.slice(anchor.length + 1)
    if (!/^khoan-[0-9]+[a-zđ0-9]*$/i.test(member) || seen.has(identity.anchor)) return []
    const supplied = (identity as typeof identity & { citation?: string }).citation
    const citation = supplied || citationFromAnchor(identity.anchor)
    if (!citation) return []
    seen.add(identity.anchor)
    return [citation]
  })
}
