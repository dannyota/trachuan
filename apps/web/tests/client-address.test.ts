import { expect, it } from 'vitest'
import { clientAddress } from '../e2e/client-address'

it('assigns distinct documentation addresses to test executions and retries', () => {
  const addresses = new Set<string>()
  for (let workerIndex = 0; workerIndex < 12; workerIndex++) {
    for (let repeatEachIndex = 0; repeatEachIndex < 3; repeatEachIndex++) {
      for (let retry = 0; retry < 3; retry++) {
        for (let testIndex = 0; testIndex < 7; testIndex++) {
          const address = clientAddress({ workerIndex, repeatEachIndex, retry, testIndex })
          expect(address).toMatch(/^(192\.0\.2|198\.51\.100|203\.0\.113)\.\d+$/)
          expect(addresses.has(address)).toBe(false)
          addresses.add(address)
        }
      }
    }
  }
  expect(addresses.size).toBe(756)
})

it('rejects inputs outside the address allocation', () => {
  expect(() => clientAddress({ workerIndex: 12, repeatEachIndex: 0, retry: 0, testIndex: 0 })).toThrow()
})
