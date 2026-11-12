import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { expect, test } from '@playwright/test'

test('returning to topic pages through browser history renders the tree', async ({ page }) => {
  const api = createServer((request, response) => {
    if (request.url?.startsWith('/api/v1/public/taxonomy')) {
      response.writeHead(200, { 'content-type': 'application/json' })
      const node = { code: '1', name: 'Fixture topic', provisions: 4, documents: 2 }
      const data = request.url?.endsWith('/1') ? { node, path: [node], documents: [] } : { nodes: [node], truncated: false }
      response.end(JSON.stringify(data))
    } else {
      response.writeHead(401, { 'content-type': 'application/json' }).end('{}')
    }
  })
  api.listen(0, '127.0.0.1')
  await once(api, 'listening')
  const apiPort = (api.address() as { port: number }).port
  const reservation = createServer()
  reservation.listen(0, '127.0.0.1')
  await once(reservation, 'listening')
  const port = (reservation.address() as { port: number }).port
  await new Promise<void>((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()))
  const server = spawn(process.execPath, ['.output/server/index.mjs'], {
    env: { ...process.env, NITRO_HOST: '127.0.0.1', NITRO_PORT: String(port), NUXT_API_ORIGIN: `http://127.0.0.1:${apiPort}` },
    stdio: 'ignore',
  })
  const origin = `http://127.0.0.1:${port}`
  try {
    await expect.poll(async () => {
      try { return (await fetch(origin)).status } catch { return 0 }
    }).toBe(200)
    const documentRequests: string[] = []
    page.on('request', request => { if (request.isNavigationRequest()) documentRequests.push(request.url()) })
    for (const path of ['/vn/topics', '/vn/topics/1']) {
      await page.goto(`${origin}${path}`)
      await expect(page.locator('main').getByText('Fixture topic').first()).toBeVisible()
      await page.getByRole('banner').getByRole('link', { name: 'Kết nối' }).click()
      await expect(page).toHaveURL(`${origin}/connect`)
      await page.goBack()
      await expect(page.locator('main').getByText('Fixture topic').first()).toBeVisible()
      await expect(page.getByText('Dịch vụ chưa sẵn sàng.')).toHaveCount(0)
      expect(documentRequests.filter(url => url === `${origin}${path}`).length).toBeGreaterThanOrEqual(2)
    }
  } finally {
    server.kill()
    await new Promise<void>(resolve => api.close(() => resolve()))
  }
})
