export function isDocumentKey(key: string): boolean {
  return /^vn:[a-z]+:[A-Za-z0-9._-]+$/.test(key)
}
