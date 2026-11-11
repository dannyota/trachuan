import type { components } from '@trachuan/schema/gen/openapi'

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

export function readerLink(hit: Hit, release: string): string {
  const anchor = hit.handle?.split('#', 2)[1]
  return `/vn/documents/${encodeURIComponent(hit.doc || '')}?release=${encodeURIComponent(release)}${anchor ? `#${encodeURIComponent(anchor).replace(/%2F/gi, '/')}` : ''}`
}
