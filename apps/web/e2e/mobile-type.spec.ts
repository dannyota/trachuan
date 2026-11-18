import { expect, test, type Page } from '@playwright/test'
import { clientAddress } from './client-address'
import { withPublicSite } from './public-fixture'
import { withSummarySite } from './summary-fixture'
import { openAccount } from './account-fixture'

type Rule = [selector: string, base: number, mobile: number]

const widths = [[390, 2], [1366, 1]] as const
const body = 'Nội dung giả lập dùng để đo cỡ chữ, không phải văn bản pháp luật thật. '.repeat(6)
const doc = { number: '12/2099/NĐ-CP', type: 'Nghị định', issuer: 'Cơ quan giả lập', title: 'Văn bản giả lập dùng cho kiểm thử bố cục', status: { class: 'in_force', current: true }, text_source: 'source_tree', text_complete: true, source_url: 'https://vbpl.vn/fixture' }

const homeRules: Rule[] = [['.home-hero h1', 60, 30], ['.home-page h2:not(.result-card h2)', 40, 24], ['.home-intro', 18, 16]]
const pageRules: Rule[] = [['.page-content h1', 36, 24], ['.page-intro p', 17, 16], ['.page-section h2', 20, 18]]
const connectRules: Rule[] = [['.page-content h1', 36, 24], ['.connect-card h2', 20, 18], ['.connect-tools h2', 20, 18]]
const privacyRules: Rule[] = [['.page-content h1', 36, 24], ['.legal-facts h2', 20, 18]]
const topicRules: Rule[] = [['.page-content h1', 36, 24]]
const loginRules: Rule[] = [['.login-card h1', 28, 24]]
const summaryRules: Rule[] = [['.reader-identity h1', 34, 22], ['.page-section h2', 20, 18]]
const readerRules: Rule[] = [['.reader-identity h1', 34, 22], ['.reader-unit h2', 20, 18], ['.law-text', 17, 16]]
const searchRules: Rule[] = [['.search-summary h1', 20, 16], ['.result-card-body h2', 18, 15], ['.result-text', 15.5, 15]]
const accountRules: Rule[] = [['.account-heading h1', 30, 24], ['.account-card h2', 17, 16]]

const inputSizes = (page: Page, scope: string) => page.evaluate(root => Array.from(document.querySelectorAll<HTMLElement>(`${root} input:not([type=checkbox]):not([type=radio]):not([type=hidden]),${root} textarea,${root} select`)).filter(e => e.getClientRects().length).map(e => parseFloat(getComputedStyle(e).fontSize)), scope)
const push = (page: Page, path: string) => page.evaluate(p => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (x: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push(p), path)

async function stubs(page: Page) {
  await page.route('**/api/v1/me', r => r.fulfill({ json: { id: 1, email: 'test@example.invalid', csrf_token: 'fixture' } }))
  await page.route('**/api/v1/public/taxonomy**', r => r.fulfill({ json: { nodes: [{ code: '1', name: 'Chủ đề một' }] } }))
  await page.route('**/api/v1/search', r => r.fulfill({ json: { release: '20990101', abstain: false, hits: [{ handle: 'vn:vbpl:1#dieu-5', doc: 'vn:vbpl:1', citation: 'Điều 5', path: [{ heading: 'Phạm vi áp dụng' }], text: body, complete: true, nodes: ['1'] }], nodes: { 1: { name: 'Chủ đề một' } }, documents: { 'vn:vbpl:1': doc } } }))
  await page.route('**/api/v1/documents/*', r => r.fulfill({ json: { release: '20990101', item: { complete: true, document: { ...doc, text_source: 'none', text_complete: false, related: [{ dir: 'out', kind: 'legal_basis', number: '2/2099' }] }, units: [{ kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', heading: 'Phạm vi', text: `1. ${body}\na) Điểm a.` }, { kind: 'provision', anchor: 'dieu-2', label: 'Điều 2', heading: 'Áp dụng', text: body }] } } }))
}

async function check(page: Page, rules: Rule[], index: 1 | 2, label: string) {
  for (const [selector, base, mobile] of rules) {
    const target = page.locator(selector).first()
    await expect(target, `${label} ${selector}`).toBeVisible()
    await expect(target, `${label} ${selector}`).toHaveCSS('font-size', `${index === 1 ? base : mobile}px`)
  }
  if (index === 1) return
  const small = await page.evaluate(() => Array.from(document.querySelectorAll<HTMLElement>('input:not([type=checkbox]):not([type=radio]):not([type=hidden]):not([type=submit]),textarea,select')).filter(e => e.getClientRects().length && parseFloat(getComputedStyle(e).fontSize) < 16).map(e => `${e.tagName}#${e.id}.${e.className}:${getComputedStyle(e).fontSize}`))
  expect(small, `${label} small controls`).toEqual([])
}

for (const [width, index] of widths) {
  test.describe(`type at ${width}px`, () => {
    test(`public pages ${width}`, async ({ browser }, info) => {
      test.setTimeout(120000)
      await withPublicSite(true, async (origin) => {
        const context = await browser.newContext({ viewport: { width, height: 844 }, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
        const page = await context.newPage()
        for (const [path, rules] of [['/', homeRules], ['/sources', pageRules], ['/connect', connectRules], ['/privacy', privacyRules], ['/vn/topics/1', topicRules], ['/login', loginRules]] as const) {
          const response = await page.goto(origin + path)
          expect(response!.status(), path).toBeLessThan(500)
          await check(page, rules, index, path)
        }
        await context.close()
      })
    })

    test(`document summary ${width}`, async ({ browser }, info) => {
      await withSummarySite(async (origin) => {
        const context = await browser.newContext({ viewport: { width, height: 844 }, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
        const page = await context.newPage()
        await page.goto(`${origin}/vn/documents/vn:vbpl:1/summary`)
        await check(page, summaryRules, index, 'summary')
        await context.close()
      })
    })

    test(`search and reader ${width}`, async ({ page }, info) => {
      await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
      await page.setViewportSize({ width: 1366, height: 844 })
      await stubs(page)
      await page.goto('/')
      await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
      await page.setViewportSize({ width, height: 844 })
      await push(page, '/vn/search')
      await expect(page.locator('.search-idle')).toBeVisible()
      await page.locator('#search-query').fill('phạm vi áp dụng')
      await page.locator('.search-box').evaluate(f => (f as HTMLFormElement).requestSubmit())
      await expect(page.locator('.result-card').first()).toBeVisible()
      await check(page, searchRules, index, 'search')
      if (index === 2) {
        await page.getByRole('button', { name: /bộ lọc ·|filters ·/i }).click()
        await expect(page.getByRole('dialog')).toBeVisible()
        const sizes = await inputSizes(page, 'dialog')
        expect(sizes.length).toBeGreaterThan(1)
        expect(Math.min(...sizes), 'filter dialog inputs').toBeGreaterThanOrEqual(16)
        await page.keyboard.press('Escape')
      }
      await push(page, '/vn/documents/vn:vbpl:1#dieu-1')
      await expect(page.locator('.reader-unit-target')).toBeVisible()
      await check(page, readerRules, index, 'reader')
    })

    test(`account ${width}`, async ({ page }, info) => {
      await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(info) })
      await page.setViewportSize({ width, height: 844 })
      await openAccount(page)
      await check(page, accountRules, index, 'account')
      if (index === 2) {
        await page.getByRole('button', { name: /^(Create token|Tạo mã mới)$/ }).click()
        await expect(page.locator('dialog[open]')).toBeVisible()
        const sizes = await inputSizes(page, 'dialog[open]')
        expect(sizes.length).toBeGreaterThan(1)
        expect(Math.min(...sizes), 'token dialog inputs').toBeGreaterThanOrEqual(16)
      }
    })
  })
}

test('no horizontal scroll at 360px', async ({ browser }, info) => {
  test.setTimeout(120000)
  await withPublicSite(true, async (origin) => {
    const context = await browser.newContext({ viewport: { width: 360, height: 844 }, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
    const page = await context.newPage()
    for (const path of ['/', '/sources', '/connect', '/privacy', '/vn/topics/1', '/login']) {
      await page.goto(origin + path)
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)
      expect(overflow, path).toBeLessThanOrEqual(0)
    }
    await context.close()
  })
})

for (const [width, hero, section, title, law] of [[767, 30, 24, 24, 16], [768, 48, 32, 36, 17]] as const) {
  test(`breakpoint at ${width}px`, async ({ browser }, info) => {
    test.setTimeout(120000)
    await withPublicSite(true, async (origin) => {
      const context = await browser.newContext({ viewport: { width, height: 844 }, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
      const page = await context.newPage()
      await page.goto(origin + '/')
      await expect(page.locator('.home-hero h1')).toHaveCSS('font-size', `${hero}px`)
      await expect(page.locator('.home-page h2:not(.result-card h2)').first()).toHaveCSS('font-size', `${section}px`)
      await page.goto(origin + '/sources')
      await expect(page.locator('.page-content h1')).toHaveCSS('font-size', `${title}px`)
      await context.close()
    })
    const page = await browser.newPage({ viewport: { width: 1366, height: 844 }, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
    await stubs(page)
    await page.goto('/')
    await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
    await page.setViewportSize({ width, height: 844 })
    await push(page, '/vn/documents/vn:vbpl:1#dieu-1')
    await expect(page.locator('.reader-unit-target')).toBeVisible()
    await expect(page.locator('.law-text').first()).toHaveCSS('font-size', `${law}px`)
    await page.close()
  })
}

test('touch devices keep inputs at 16px on wide viewports', async ({ browser }, info) => {
  const context = await browser.newContext({ viewport: { width: 900, height: 600 }, hasTouch: true, isMobile: true, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
  const page = await context.newPage()
  expect(await page.evaluate(() => matchMedia('(pointer: coarse)').matches)).toBe(true)
  await stubs(page)
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await push(page, '/vn/search')
  const date = page.locator('.search-filters input[type=date]')
  await expect(date).toBeVisible()
  await expect(date).toHaveCSS('font-size', '16px')
  await context.close()
})

test('mouse desktops keep the filter date input at 13px', async ({ browser }, info) => {
  const context = await browser.newContext({ viewport: { width: 1366, height: 844 }, extraHTTPHeaders: { 'X-Forwarded-For': clientAddress(info) } })
  const page = await context.newPage()
  expect(await page.evaluate(() => matchMedia('(pointer: coarse)').matches)).toBe(false)
  await stubs(page)
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await push(page, '/vn/search')
  const date = page.locator('.search-filters input[type=date]')
  await expect(date).toBeVisible()
  await expect(date).toHaveCSS('font-size', '13px')
  await context.close()
})
