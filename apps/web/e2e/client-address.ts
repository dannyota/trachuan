import { createHash } from 'node:crypto'

interface AddressInput {
  testId: string
  repeatEachIndex: number
  retry: number
}

export function clientAddress({ testId, repeatEachIndex, retry }: AddressInput): string {
  const digest = createHash('sha256').update(JSON.stringify([testId, repeatEachIndex, retry])).digest('hex').slice(-24)
  return `2001:db8:${digest.match(/.{4}/g)!.join(':')}`
}
