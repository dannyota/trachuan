const publicPaths = new Set(['/', '/connect', '/sources', '/privacy', '/terms', '/llms.txt'])
const privatePaths = new Set(['/login', '/account', '/vn/search'])

export function matchPage(url: string): { path: string, public: boolean, alias: boolean } | null {
  const raw = url.split('?')[0] ?? ''
  if (!raw.startsWith('/') || raw.includes('%')) return null
  const path = raw.replace(/\/{2,}/g, '/').replace(/\/+$/, '') || '/'
  const documentKey = /^\/vn\/documents\/([^/]+)$/.exec(path)?.[1]
  const document = documentKey !== undefined && documentKey !== '.' && documentKey !== '..'
  if (!publicPaths.has(path) && !privatePaths.has(path) && !document) return null
  return { path, public: publicPaths.has(path), alias: raw !== path }
}
