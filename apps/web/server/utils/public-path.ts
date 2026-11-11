import { posix } from 'node:path'

const paths = new Set(['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt'])

export function publicPath(url: string): string | null {
  const raw = url.split('?')[0] ?? ''
  let decoded: string
  try {
    decoded = decodeURIComponent(raw.replace(/%25/g, '%2525').replace(/%2f/gi, '%252F')).split('?')[0] ?? ''
  } catch {
    return null
  }
  if (!decoded.startsWith('/')) return null
  const normalized = posix.normalize(decoded).replace(/\/+$/, '') || '/'
  return paths.has(normalized) ? normalized : null
}
