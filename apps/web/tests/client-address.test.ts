import { expect, it } from 'vitest'
import { clientAddress } from '../e2e/client-address'

it('derives stable IPv6 documentation addresses without worker or repeat limits', () => {
  const input = { testId: 'suite:test', repeatEachIndex: 100, retry: 300 }
  const address = clientAddress(input)
  expect(address).toBe('2001:db8:c388:f08d:4e15:5fc7:5bc4:7289')
  expect(clientAddress(input)).toBe(address)
  expect(clientAddress({ ...input, testId: 'suite:other' })).not.toBe(address)
  expect(clientAddress({ ...input, repeatEachIndex: 101 })).not.toBe(address)
  expect(clientAddress({ ...input, retry: 301 })).not.toBe(address)
})
