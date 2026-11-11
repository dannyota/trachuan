import { expect, test } from '@playwright/test'

test('account response is no-store', async ({ request }) => {
  const response = await request.get('/account', { maxRedirects: 0, headers: { cookie: 'account-fixture=1' } })
  expect(response.status()).toBe(200)
  expect(response.headers()['cache-control']).toBe('no-store')
})
