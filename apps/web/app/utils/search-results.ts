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
