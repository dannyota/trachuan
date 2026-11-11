export function containsPrivateValue(value: unknown, secret: string): boolean {
  const encoded = JSON.stringify(secret).slice(1, -1)
  const seen = new WeakSet<object>()
  function visit(item: unknown): boolean {
    if (typeof item === 'string') return item.includes(secret) || item.includes(encoded)
    if (!item || typeof item !== 'object' || seen.has(item)) return false
    seen.add(item)
    return Object.entries(item).some(([key, nested]) => visit(key) || visit(nested))
  }
  return visit(value)
}
