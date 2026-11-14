import { expect, it, vi } from 'vitest'
import { takeLinkToken, qrGeometry, recoveryText, reauthDecision } from '../app/utils/account-security'
import { encode } from 'uqr'

it('removes the fragment before exposing a token to a caller', () => {
  let url = '/link#synthetic-token'
  const replaceState = vi.fn((_state, _title, path) => { url = path })
  const token = takeLinkToken({ hash: '#synthetic-token', pathname: '/link', search: '' }, { state: { position: 1 }, replaceState })
  expect(url).toBe('/link')
  expect(token).toBe('synthetic-token')
  expect(replaceState).toHaveBeenCalledWith(null, '', '/link')
})
it('drops query parameters as well as the fragment from a link', () => {
  const replaceState = vi.fn()
  expect(takeLinkToken({ hash: '', pathname: '/link', search: '?token=untrusted' }, { state: null, replaceState })).toBe('')
  expect(replaceState).toHaveBeenCalledWith(null, '', '/link')
})
it('draws only matrix modules inside a four module quiet zone', () => {
  expect(qrGeometry([[true, false], [false, true]])).toEqual({ size: 10, path: 'M4 4h1v1h-1zM5 5h1v1h-1z' })
  const matrix = encode('otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP', { border: 0 }).data
  const geometry = qrGeometry(matrix)
  expect(geometry.size).toBe(matrix.length + 8)
  expect((geometry.path.match(/M/g) || []).length).toBe(matrix.flat().filter(Boolean).length)
})
it('formats recovery codes in groups of five, one per line', () => {
  expect(recoveryText(['abcdefghijklmnopqrstuvwxyz', 'ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z'])).toBe('ABCDE FGHIJ KLMNO PQRST UVWXY Z\nABCDE FGHIJ KLMNO PQRST UVWXY Z\n')
})
it('chooses password reauth and only offers factor entry on a route that accepts it', () => {
  const error = { error: { code: 'reauth_required' }, factor: true }
  expect(reauthDecision({ password: true, google: true }, error, true)).toEqual({ kind: 'password', factor: true })
  expect(reauthDecision({ password: false, google: true }, error, true)).toEqual({ kind: 'login', factor: false })
  expect(reauthDecision({ password: true, google: false }, error, false)).toEqual({ kind: 'login', factor: false })
  expect(reauthDecision({ password: true, google: false }, { error: { code: 'reauth_required' } }, false)).toEqual({ kind: 'password', factor: false })
  expect(reauthDecision({ password: true, google: false }, { error: { code: 'csrf_rejected' } }, true)).toBeNull()
})

it('does not retain the fragment in router history state', () => {
  const replaceState = vi.fn()
  takeLinkToken({ hash: '#synthetic-token', pathname: '/link', search: '' }, { state: { current: '/link#synthetic-token', position: 3 }, replaceState })
  expect(JSON.stringify(replaceState.mock.calls)).not.toContain('synthetic-token')
})
