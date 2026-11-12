import type { components } from '@trachuan/schema/gen/openapi'
import { isDocumentKey } from './document-key'

type Hit = components['schemas']['Hit']
type Result = components['schemas']['SearchResult']

export function validityInk(status: string | undefined): string {
  switch (status) {
    case 'in_force': return 'in-force'
    case 'partial': return 'partial'
    case 'not_yet': return 'not-yet'
    case 'expired': return 'expired'
    default: return 'neutral'
  }
}

export function resultHits(result: Result): Hit[] {
  return result.abstain ? [] : [...(result.hits || []), ...(result.non_current || [])]
}

export function readerLink(hit: Hit, release: string): string | null {
  if (!hit.doc || !isDocumentKey(hit.doc)) return null
  const anchor = hit.handle?.split('#', 2)[1]
  return `/vn/documents/${hit.doc}?release=${encodeURIComponent(release)}${anchor ? `#${encodeURI(anchor)}` : ''}`
}


type Warning = components['schemas']['Warning']
type Messages = ReturnType<typeof import('../../i18n').messagesFor>

export function warningVerb(kind: string | undefined, messages: Messages): string {
  switch (kind) {
    case 'repeals': return messages.warningRepeals
    case 'replaces': return messages.warningReplaces
    case 'suspends': return messages.warningSuspends
    case 'amends_supplements': return messages.warningAmends
    default: return kind || ''
  }
}

export function warningTarget(warning: Warning): { text: string, raw: boolean } {
  if (warning.exact) return { text: warning.citation, raw: false }
  const path = warning.named || ''
  const match = /^dieu-([0-9]+)(?:\/khoan-([0-9]+))?(?:\/diem-([a-zđ]))?$/.exec(path)
  if (!match || (match[3] && !match[2])) return { text: path, raw: true }
  return { text: [`Điều ${match[1]}`, ...(match[2] ? [`khoản ${match[2]}`] : []), ...(match[3] ? [`điểm ${match[3]}`] : [])].join(', '), raw: false }
}

export function warningLink(warning: Warning, release: string): string | null {
  if (!warning.in_release || !warning.key || !isDocumentKey(warning.key)) return null
  const prefix = `${warning.key}#`
  const anchor = warning.source?.startsWith(prefix) ? warning.source.slice(prefix.length) : ''
  return `/vn/documents/${warning.key}?release=${encodeURIComponent(release)}${anchor ? `#${encodeURI(anchor)}` : ''}`
}
