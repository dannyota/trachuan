type LinkLocation = Pick<Location, 'hash' | 'pathname' | 'search'>
type LinkHistory = Pick<History, 'state' | 'replaceState'>

export function takeLinkToken(location: LinkLocation, history: LinkHistory, routeHistory?: { replace: (path: string) => void }): string {
  const token = location.hash.slice(1)
  history.replaceState(null, '', location.pathname)
  routeHistory?.replace(location.pathname)
  return token
}

export function qrGeometry(matrix: boolean[][]): { size: number, path: string } {
  return {
    size: matrix.length + 8,
    path: matrix.flatMap((row, y) => row.flatMap((filled, x) => filled ? [`M${x + 4} ${y + 4}h1v1h-1z`] : [])).join(''),
  }
}

export function recoveryText(codes: string[]): string {
  return codes.map(code => code.replace(/[\s-]/g, '').toUpperCase().match(/.{1,5}/g)?.join(' ') || '').join('\n') + '\n'
}

type Refusal = { error?: { code?: string }, factor?: boolean }
export function reauthDecision(account: { password: boolean, google: boolean } | null, error: Refusal, acceptsCode: boolean): { kind: 'password' | 'login', factor: boolean } | null {
  if (error.error?.code !== 'reauth_required') return null
  if (!account?.password || (error.factor && !acceptsCode)) return { kind: 'login', factor: false }
  return { kind: 'password', factor: error.factor === true }
}
