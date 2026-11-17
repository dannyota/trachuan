export function frameworkItemError(code: string | undefined): 'restart' | 'not-found' | 'unit_too_large' | 'invalid_item' | undefined {
  if (code === 'invalid_next' || code === 'release_expired') return 'restart'
  if (code === 'invalid_handle' || code === 'not_found') return 'not-found'
  if (code === 'unit_too_large' || code === 'invalid_item') return code
}
