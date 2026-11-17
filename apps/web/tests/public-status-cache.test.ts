import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { publicStatus } from '../server/utils/public-status'

type Deferred = { resolve: (response: Response) => void, reject: (reason: Error) => void }
let requests: Deferred[]
let now: number

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
}

beforeEach(() => {
  now = 100_000
  requests = []
  vi.spyOn(Date, 'now').mockImplementation(() => now)
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve, reject) => requests.push({ resolve, reject }))))
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('shares a cold in-flight request', async () => {
  const first = publicStatus('http://cold.invalid')
  const second = publicStatus('http://cold.invalid')
  await Promise.resolve()
  expect(requests).toHaveLength(1)
  requests[0]!.resolve(response({ release: 'A' }))
  expect(await Promise.all([first, second])).toEqual([{ release: 'A' }, { release: 'A' }])
})

it('shares an expired in-flight request', async () => {
  const origin = 'http://expired.invalid'
  const first = publicStatus(origin)
  await Promise.resolve()
  requests[0]!.resolve(response({ release: 'A' }))
  await first
  now += 30_000
  const second = publicStatus(origin)
  const third = publicStatus(origin)
  await Promise.resolve()
  expect(requests).toHaveLength(2)
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await Promise.all([second, third])).toEqual([{ release: 'B' }, { release: 'B' }])
})

it('counts the lifetime from the start of the request', async () => {
  const origin = 'http://lifetime.invalid'
  const first = publicStatus(origin)
  await Promise.resolve()
  now += 10_000
  requests[0]!.resolve(response({ release: 'A' }))
  await first
  now += 19_999
  expect(await publicStatus(origin)).toEqual({ release: 'A' })
  expect(requests).toHaveLength(1)
  now += 1
  const next = publicStatus(origin)
  await Promise.resolve()
  expect(requests).toHaveLength(2)
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await next).toEqual({ release: 'B' })
})

it('requires a stored request to start strictly after the threshold', async () => {
  const origin = 'http://strict-value.invalid'
  const first = publicStatus(origin)
  await Promise.resolve()
  requests[0]!.resolve(response({ release: 'A' }))
  await first
  expect(await publicStatus(origin, now - 1)).toEqual({ release: 'A' })
  expect(requests).toHaveLength(1)
  const next = publicStatus(origin, now)
  await Promise.resolve()
  expect(requests).toHaveLength(2)
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await next).toEqual({ release: 'B' })
})

it('shares an in-flight request started after the threshold', async () => {
  const origin = 'http://strict-pending-newer.invalid'
  const first = publicStatus(origin)
  const second = publicStatus(origin, now - 1)
  await Promise.resolve()
  expect(requests).toHaveLength(1)
  requests[0]!.resolve(response({ release: 'A' }))
  expect(await Promise.all([first, second])).toEqual([{ release: 'A' }, { release: 'A' }])
})

it.each([0, 1])('does not share an in-flight request started at or before the threshold, offset %s', async (offset) => {
  const origin = `http://strict-pending-${offset}.invalid`
  const first = publicStatus(origin)
  await Promise.resolve()
  const second = publicStatus(origin, now + offset)
  await Promise.resolve()
  expect(requests).toHaveLength(2)
  requests[0]!.resolve(response({ release: 'A' }))
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await Promise.all([first, second])).toEqual([{ release: 'A' }, { release: 'B' }])
})

it('keeps the replacement when the older request fails', async () => {
  const origin = 'http://replacement-failure.invalid'
  const first = publicStatus(origin)
  await Promise.resolve()
  const second = publicStatus(origin, now)
  await Promise.resolve()
  requests[0]!.reject(new Error('failed'))
  expect(await first).toBeUndefined()
  const third = publicStatus(origin)
  await Promise.resolve()
  expect(requests).toHaveLength(2)
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await Promise.all([second, third])).toEqual([{ release: 'B' }, { release: 'B' }])
  expect(await publicStatus(origin)).toEqual({ release: 'B' })
  expect(requests).toHaveLength(2)
})

it('keeps the replacement when the older request succeeds last', async () => {
  const origin = 'http://replacement-success.invalid'
  const first = publicStatus(origin)
  await Promise.resolve()
  const second = publicStatus(origin, now)
  await Promise.resolve()
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await second).toEqual({ release: 'B' })
  requests[0]!.resolve(response({ release: 'A' }))
  expect(await first).toEqual({ release: 'A' })
  expect(await publicStatus(origin)).toEqual({ release: 'B' })
  expect(requests).toHaveLength(2)
})

it.each(['429', 'non-object', 'invalid-release', 'throw'])('does not store a %s failure', async (failure) => {
  const origin = `http://failure-${failure}.invalid`
  const first = publicStatus(origin)
  await Promise.resolve()
  if (failure === 'throw') requests[0]!.reject(new Error('failed'))
  else if (failure === '429') requests[0]!.resolve(response({}, 429))
  else if (failure === 'non-object') requests[0]!.resolve(response('invalid'))
  else requests[0]!.resolve(response({ release: 1 }))
  expect(await first).toBeUndefined()
  const next = publicStatus(origin)
  await Promise.resolve()
  expect(requests).toHaveLength(2)
  requests[1]!.resolve(response({ release: 'B' }))
  expect(await next).toEqual({ release: 'B' })
})
