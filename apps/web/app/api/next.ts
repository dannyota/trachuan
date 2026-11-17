import { isNodeCode } from '../utils/node-code'

function valid(value: string): boolean {
  if (!value.startsWith('/') || value.startsWith('//') || [...value].some(char => char === '\\' || char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) return false
  return !value.split('?')[0]!.split('/').some(segment => segment === '.' || segment === '..')
}

export function safeNext(input: string): string {
  const withoutFragment = input.split('#', 1)[0]!
  const separator = withoutFragment.indexOf('?')
  const path = separator < 0 ? withoutFragment : withoutFragment.slice(0, separator)
  const query = separator < 0 ? '' : withoutFragment.slice(separator + 1)
  let result = path || ''
  if (result === '/vn/search') {
    const node = new URLSearchParams(query).get('node')
    if (node && isNodeCode(node)) result += `?node=${node}`
  }
  if (/^\/vn\/documents\/[^/]+$/.test(result)) {
    const release = new URLSearchParams(query).get('release')
    if (release && /^[0-9]{8}(?:_[0-9]+)?$/.test(release)) result += `?release=${release}`
  }
  if (/^\/frameworks\/[^/]+\/[^/]+\/control$/.test(result)) {
    const citation = new URLSearchParams(query).get('citation')
    if (citation !== null) result += `?${new URLSearchParams({ citation })}`
  }
  if (new TextEncoder().encode(result).length > 512 || !valid(result)) return '/'
  try {
    return valid(decodeURIComponent(result)) ? result : '/'
  } catch {
    return '/'
  }
}

export function loginUrl(path: string): string {
  return `/login?next=${encodeURIComponent(safeNext(path))}`
}
