import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createInterface } from 'node:readline'
import { expect, test, type Page } from '@playwright/test'
import { openAccount } from './account-fixture'
import { clientAddress } from './client-address'

test.beforeEach(async ({ page }, testInfo) => { await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) }) })

async function withTopicSite(run: (origin: string) => Promise<void>) {
  const api = createServer((request, response) => {
    const node = { code: '1', name: 'Fixture topic', provisions: 4, documents: 2 }
    const data = request.url?.startsWith('/api/v1/public/taxonomy') ? { node, path: [node], documents: [] } : {}
    response.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(data))
  })
  api.listen(0, '127.0.0.1')
  await once(api, 'listening')
  const port = (api.address() as { port: number }).port
  const server = spawn(process.execPath, ['.output/server/index.mjs'], {
    env: { ...process.env, NITRO_HOST: '127.0.0.1', NITRO_PORT: '0', NUXT_API_ORIGIN: `http://127.0.0.1:${port}` },
    stdio: ['ignore', 'pipe', 'ignore'],
  })
  try {
    const origin = await new Promise<string>((resolve, reject) => {
      server.once('exit', code => reject(new Error(`site exited with code ${code}`)))
      createInterface({ input: server.stdout }).on('line', line => {
        const match = /Listening on (http:\/\/127\.0\.0\.1:\d+)/.exec(line)
        if (match) resolve(match[1]!)
      })
    })
    await run(origin)
  } finally {
    server.kill()
    await new Promise<void>(resolve => api.close(() => resolve()))
  }
}

async function checkChrome(page: Page, path: string, colorScheme: 'light' | 'dark', footerBand = true) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), path).toBe(true)
  const pills = page.locator('.btn:visible:not(.google-button), .chip:visible, .topic-tree button:visible')
  for (const pill of await pills.all()) {
    const dimensions = await pill.evaluate(el => ({ height: el.getBoundingClientRect().height, radius: parseFloat(getComputedStyle(el).borderRadius) }))
    expect(dimensions.height, path).toBeGreaterThanOrEqual(44)
    expect(dimensions.radius, path).toBeGreaterThanOrEqual(dimensions.height / 2)
  }
  await page.keyboard.press('Tab')
  expect(await page.evaluate(() => {
    const element = document.activeElement
    if (!(element instanceof HTMLElement) || !element.matches(':focus-visible')) return false
    const style = getComputedStyle(element)
    return style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) >= 3
  }), path).toBe(true)
  if (!footerBand) return
  const footer = page.locator('footer.site-footer')
  await expect(footer, path).toBeVisible()
  await expect(footer, path).toHaveCSS('background-color', colorScheme === 'light' ? 'rgb(15, 21, 36)' : 'rgb(19, 26, 46)')
}

for (const colorScheme of ['light', 'dark'] as const) for (const width of [360, 390, 768, 1366]) {
  test(`chrome fits at ${width}px in ${colorScheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme })
    await page.setViewportSize({ width, height: 900 })
    await page.route('**/api/v1/auth/factor', route => route.fulfill({ json: { csrf: 'csrf', next: '/account' } }))
    await page.route('**/api/v1/auth/link', route => route.fulfill({ json: { purpose: 'registration', email: 'test@example.invalid' } }))
    for (const path of ['/login', '/signup', '/reset', '/link#synthetic', '/login/factor', '/connect', '/sources', '/privacy', '/terms']) {
      await page.goto(path)
      if (path.startsWith('/link')) await expect(page.locator('input[name="new-password"]')).toBeVisible()
      if (path === '/login/factor') await expect(page.locator('input[autocomplete="one-time-code"]')).toBeVisible()
      await checkChrome(page, path, colorScheme)
    }
    await openAccount(page)
    await checkChrome(page, '/account', colorScheme)
    await withTopicSite(async origin => {
      await page.goto(`${origin}/vn/topics/1`)
      await expect(page.locator('main').getByText('Fixture topic').first()).toBeVisible()
      await checkChrome(page, '/vn/topics/1', colorScheme)
    })
    const response = await page.goto('/no-such-page')
    expect(response?.status()).toBe(404)
    await checkChrome(page, '/no-such-page', colorScheme, false)
  })
}
