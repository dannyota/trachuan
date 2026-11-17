const alpha = 'fw:fw-alpha:2.1'
const beta = 'fw:fw-beta:1.0'
const base = { release: '20990101', access_set_version: 'synthetic', retrieval_recipe_status: 'measured' }
const counts = { controls: 1, edges_resolved: 0, edges_unresolved: 0 }
const version = (code, number, readable) => ({ code, version: number, name: `Synthetic ${code}`, publisher: 'Synthetic publisher', access: code === 'fw-alpha' ? 'licensed' : 'free', readable })
const versionStatus = (readable, access) => ({ ...counts, readable, access, ...(readable ? { sources: [{ name: 'Synthetic source', text_source: 'synthetic', text_complete: true }] } : {}) })
const error = code => ({ error: { code, message: code } })
export function frameworkFixture(request, response) {
  const url = new URL(request.url, 'http://fixture.invalid')
  if (!url.pathname.startsWith('/api/v1/frameworks')) return false
  const cookies = request.headers.cookie || ''
  if (!cookies.includes('framework-fixture=')) return false
  const mode = /framework-fixture=([^;]+)/.exec(cookies)?.[1] || 'mixed'
  const send = (status, value) => { response.writeHead(status, { 'content-type': 'application/json', 'cache-control': 'no-store' }); response.end(JSON.stringify(value)); return true }
  if (mode === 'off') return send(404, error('not_found'))
  const alphaReadable = mode === 'full'
  const betaReadable = true
  const versions = { [alpha]: versionStatus(alphaReadable, 'licensed'), [beta]: versionStatus(betaReadable, 'free') }
  const frameworks = { [alpha]: version('fw-alpha', '2.1', alphaReadable), [beta]: version('fw-beta', '1.0', betaReadable) }
  if (mode === 'reduced') { delete versions[beta]; delete frameworks[beta] }
  if (url.pathname === '/api/v1/frameworks' && request.method === 'GET') return send(200, { ...base, versions, totals: { controls: mode === 'reduced' ? 1 : 2, edges_resolved: 0, edges_unresolved: 0 }, ...(mode === 'unavailable' ? { access_unavailable: true } : {}) })
  if (url.pathname === '/api/v1/frameworks/gaps') return send(200, { ...base, gaps: [{ kind: 'title_missing', version: alpha, detail: 'Synthetic gap' }] })
  if (url.pathname.startsWith('/api/v1/frameworks/versions/')) {
    const handle = decodeURIComponent(url.pathname.slice('/api/v1/frameworks/versions/'.length))
    const entry = frameworks[handle]
    if (!entry) return send(200, { ...base, items: [{ error: 'not_found', complete: false }] })
    return send(200, { ...base, items: [{ handle, complete: true, units: [{ kind: 'head', version: entry }, { kind: 'outline', outline: { citation: 'A (1)/#%', kind: 'control', title: 'Synthetic title' } }] }] })
  }
  if (url.pathname.startsWith('/api/v1/frameworks/controls/')) {
    const handle = decodeURIComponent(url.pathname.slice('/api/v1/frameworks/controls/'.length))
    const prefix = handle.split('#')[0]
    const entry = frameworks[prefix]
    if (!entry) return send(200, { ...base, items: [{ error: 'not_found', complete: false }] })
    const head = { handle, citation: 'A (1)/#%', kind: 'control', title: 'Synthetic title', access: entry.access, text_withheld: !entry.readable }
    const text = entry.readable ? [{ kind: 'title_original', text: 'Synthetic publisher title' }, { kind: 'body', text: 'Synthetic publisher body' }] : []
    return send(200, { ...base, items: [{ handle, complete: true, units: [{ kind: 'head', head }, ...text] }] })
  }
  if (url.pathname === '/api/v1/frameworks/search' && request.method === 'POST') {
    const hit = (handle, entry) => ({ handle: `${handle}#A`, citation: 'A', kind: 'control', title: 'Synthetic title', mappings: 0, text_withheld: !entry.readable, lookup: true, ...(entry.readable ? { snippet: 'Synthetic snippet' } : {}) })
    return send(200, { ...base, abstain: false, frameworks, hits: Object.entries(frameworks).map(([handle, entry]) => hit(handle, entry)) })
  }
  return send(404, error('not_found'))
}
