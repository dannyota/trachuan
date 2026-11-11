import { BlockList, isIP } from 'node:net'

function canonical(address: string): string | null {
  const kind = isIP(address)
  if (kind === 4) return address
  if (kind !== 6 || address.includes('%')) return null
  const host = new URL(`http://[${address}]/`).hostname.slice(1, -1)
  const mapped = /^::ffff:([\da-f]{1,4}):([\da-f]{1,4})$/.exec(host)
  if (mapped) {
    const high = parseInt(mapped[1]!, 16)
    const low = parseInt(mapped[2]!, 16)
    return `${high >> 8}.${high & 255}.${low >> 8}.${low & 255}`
  }
  return host
}

export function clientAddress(peer: string, forwarded: string, ranges: string[]): string {
  const client = canonical(peer) ?? 'unknown'
  const list = new BlockList()
  for (const range of ranges) {
    const [address, bits, extra] = range.trim().split('/')
    const kind = isIP(address ?? '')
    if (!kind || bits === undefined || extra !== undefined || !/^\d+$/.test(bits)) continue
    const prefix = Number(bits)
    if (prefix > (kind === 4 ? 32 : 128)) continue
    list.addSubnet(address!, prefix, kind === 4 ? 'ipv4' : 'ipv6')
  }
  const trusted = (address: string) => {
    const kind = isIP(address)
    return kind !== 0 && list.check(address, kind === 4 ? 'ipv4' : 'ipv6')
  }
  if (!trusted(client)) return client
  for (const item of forwarded.split(',').reverse()) {
    const address = canonical(item.trim())
    if (!address) return client
    if (!trusted(address)) return address
  }
  return client
}

export function createAdmission(clock: () => number) {
  const buckets = new Map<string, { start: number, count: number }>()
  let active = 0
  return {
    allow(address: string): boolean {
      const now = clock()
      let bucket = buckets.get(address)
      if (!bucket || now - bucket.start >= 60_000) {
        if (buckets.size >= 100_000) {
          for (const [key, value] of buckets) {
            if (now - value.start >= 60_000) buckets.delete(key)
          }
          if (buckets.size >= 100_000 && !buckets.has(address)) return false
        }
        bucket = { start: now, count: 0 }
        buckets.set(address, bucket)
      }
      if (bucket.count >= 120) return false
      bucket.count++
      return true
    },
    render(): (() => void) | null {
      if (active >= 4) return null
      active++
      let released = false
      return () => {
        if (!released) active--
        released = true
      }
    },
  }
}
