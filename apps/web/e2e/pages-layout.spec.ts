import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { expect, test } from '@playwright/test'

test('connect keeps code scrollable and displays numbered steps at each width', async ({ page }) => {
  await page.goto('/connect')
  for (const [width, columns] of [[390, 1], [1023, 1], [1024, 2]] as const) {
    await page.setViewportSize({ width, height: 900 })
    const grid = page.locator('.connect-grid')
    expect(await grid.evaluate(el => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(columns)
    await expect(page.locator('.connect-card ol').first()).toHaveCSS('list-style-type', 'decimal')
    await expect(page.locator('.server-address pre')).toHaveCSS('font-size', '16px')
    for (const block of await page.locator('.page-code pre').all()) {
      await expect(block).toHaveCSS('white-space', 'pre')
      await expect(block).toHaveCSS('overflow-x', 'auto')
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('sources shows four coverage facts in a two by two grid on mobile', async ({ page }) => {
  const api = createServer((_request, response) => {
    response.writeHead(200, { 'content-type': 'application/json' })
    response.end(JSON.stringify({ release: '20990101', documents: 12, provisions: 34, nodes: 2, newest_source_date: '2099-01-01', gaps: {} }))
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
    await page.setViewportSize({ width: 390, height: 900 })
    await page.goto(`${origin}/sources`)
    const facts = page.locator('.coverage-stats > div')
    await expect(facts).toHaveCount(4)
    const positions = await facts.evaluateAll(elements => elements.map(el => ({ x: el.getBoundingClientRect().x, y: el.getBoundingClientRect().y })))
    expect(positions[0]!.y).toBe(positions[1]!.y)
    expect(positions[2]!.y).toBe(positions[3]!.y)
    expect(positions[2]!.y).toBeGreaterThan(positions[0]!.y)
    expect(positions[1]!.x).toBeGreaterThan(positions[0]!.x)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  } finally {
    server.kill()
    await new Promise<void>(resolve => api.close(() => resolve()))
  }
})
