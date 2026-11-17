import { safeNext } from '../api/next'

const codePattern = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/
const versionPattern = /^[0-9a-z]+([.+-][0-9a-z]+)*$/
const citationPattern = /^[!-~]+(?: [!-~]+)*$/

export function versionHandle(code: string, version: string): string | undefined {
  return codePattern.test(code) && versionPattern.test(version) ? `fw:${code}:${version}` : undefined
}

export function controlHandle(code: string, version: string, citation: string): string | undefined {
  const versionId = versionHandle(code, version)
  if (!versionId || citation.length > 64 || !citationPattern.test(citation)) return undefined
  const encoded = Array.from(citation, char => /[A-Za-z0-9.()_~-]/.test(char) ? char : `%${char.charCodeAt(0).toString(16).toUpperCase().padStart(2, '0')}`).join('')
  return `${versionId}#${encoded}`
}

export function versionPath(handle: string): string | undefined {
  const match = /^fw:([^:#]+):([^:#]+)(?:#.*)?$/.exec(handle)
  return match && versionHandle(match[1]!, match[2]!) ? `/frameworks/${match[1]}/${match[2]}` : undefined
}

export function controlPath(handle: string, citation: string): string | undefined {
  const path = versionPath(handle)
  if (!path) return undefined
  return `${path}/control?${new URLSearchParams({ citation })}`
}

export function frameworkNext(path: string): string {
  const version = /^(\/frameworks\/[^/]+\/[^/?]+)\/control(?:\?|$)/.exec(path)?.[1]
  const citation = version ? new URLSearchParams(path.split('?')[1] || '').get('citation') : null
  const control = version && citation !== null ? `${version}/control?${new URLSearchParams({ citation })}` : path
  for (const candidate of [control, version, '/frameworks']) {
    if (candidate && safeNext(candidate) === candidate) return candidate
  }
  return '/frameworks'
}
