import { expect, it, vi } from 'vitest'
import { fulfillFrameworkFixture } from '../e2e/framework-route'

function setup() {
  const body = Buffer.from('synthetic fixture')
  const response = { status: () => 200, headers: () => ({ 'content-type': 'application/json' }), body: vi.fn(async () => body) }
  const route = { request: () => ({ url: () => 'http://127.0.0.1:3031/api/v1/frameworks?x=1' }), fetch: vi.fn(async () => response), fulfill: vi.fn(async () => {}) }
  const page = { isClosed: vi.fn(() => false) }
  return { route, response, page, body }
}

it('fulfills from the fetched bytes and preserves status and headers', async () => {
  const { route, response, page, body } = setup()
  await fulfillFrameworkFixture(route, page)
  expect(route.fetch).toHaveBeenCalledWith({ url: 'http://127.0.0.1:3032/api/v1/frameworks?x=1' })
  expect(response.body).toHaveBeenCalledOnce()
  expect(route.fulfill).toHaveBeenCalledWith({ status: 200, headers: { 'content-type': 'application/json' }, body })
})

it('ignores a failed fetch when the page has closed', async () => {
  const { route, page } = setup()
  route.fetch.mockImplementation(async () => { page.isClosed.mockReturnValue(true); throw new Error('closed') })
  await expect(fulfillFrameworkFixture(route, page)).resolves.toBeUndefined()
})

it('propagates fixture failures while the page is open', async () => {
  const { route, page } = setup()
  route.fetch.mockRejectedValue(new Error('fixture failed'))
  await expect(fulfillFrameworkFixture(route, page)).rejects.toThrow('fixture failed')
})

it('ignores a failed fulfill if the page closes during fulfillment', async () => {
  const { route, page } = setup()
  route.fulfill.mockImplementation(async () => { page.isClosed.mockReturnValue(true); throw new Error('closed') })
  await expect(fulfillFrameworkFixture(route, page)).resolves.toBeUndefined()
})

it('propagates failed fulfillment while the page is open', async () => {
  const { route, page } = setup()
  route.fulfill.mockRejectedValue(new Error('fulfill failed'))
  await expect(fulfillFrameworkFixture(route, page)).rejects.toThrow('fulfill failed')
})

it('does not fetch after the page has closed', async () => {
  const { route, page } = setup()
  page.isClosed.mockReturnValue(true)
  await fulfillFrameworkFixture(route, page)
  expect(route.fetch).not.toHaveBeenCalled()
})
