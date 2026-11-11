interface AddressInput {
  workerIndex: number
  repeatEachIndex: number
  retry: number
  testIndex: number
}

export function clientAddress(input: AddressInput): string {
  const { workerIndex, repeatEachIndex, retry, testIndex } = input
  if (![workerIndex, repeatEachIndex, retry, testIndex].every(Number.isInteger)
    || workerIndex < 0 || workerIndex >= 12
    || repeatEachIndex < 0 || repeatEachIndex >= 3
    || retry < 0 || retry >= 3
    || testIndex < 0 || testIndex >= 7) {
    throw new Error('Playwright client address allocation exceeded')
  }
  const slot = (((workerIndex * 3) + repeatEachIndex) * 3 + retry) * 7 + testIndex
  const prefixes = ['192.0.2', '198.51.100', '203.0.113']
  return `${prefixes[Math.floor(slot / 256)]}.${slot % 256}`
}
