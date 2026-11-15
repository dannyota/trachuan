import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createInterface } from 'node:readline'
import { expect, test, type Page } from '@playwright/test'
import { openAccount } from './account-fixture'
import { clientAddress } from './client-address'

const allowlist: { selector: string, reason: string }[] = []
const status = { release: '20990101', documents: 1234567, provisions: 9876543, nodes: 2, newest_source_date: '2099-12-28', gaps: {} }
const account = { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'csrf', password: true, google: false, totp: false, totp_enrollment: true }
const node = { code: '1', name: 'Fixture topic', provisions: 4, documents: 2 }
const codes = Array(10).fill('ABCDEFGHIJKLMNOPQRSTUVWXYZ')

async function publicSite(run: (origin: string) => Promise<void>) {
  const api = createServer((request, response) => {
    const data = request.url?.startsWith('/api/v1/public/taxonomy')
      ? request.url.endsWith('/1') ? { node, path: [node], documents: [] } : { nodes: [node], truncated: false }
      : status
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

async function navigate(page: Page, path: string) {
  await page.evaluate(path => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push(path), path)
}

async function accountState(page: Page, state: string, language: string) {
  await openAccount(page)
  const password = !['unset', 'add notice'].includes(state)
  const totp = state === 'remove'
  await page.route('**/api/v1/me', route => route.fulfill({ json: { ...account, password, google: !password, totp } }))
  await page.context().addCookies([
    { name: 'lang', value: language, url: 'http://127.0.0.1:3031' },
    { name: 'account-security', value: `${Number(password)}${Number(totp)}1`, url: 'http://127.0.0.1:3031' },
  ])
  await page.reload()
  await expect(page.locator('.account-security')).toBeVisible()
  if (state === 'setup' || state === 'recovery') {
    await page.route('**/api/v1/me/totp/setup', route => route.fulfill({ json: { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/synthetic?secret=JBSWY3DPEHPK3PXP' } }))
    await page.route('**/api/v1/me/totp/confirm', route => route.fulfill({ json: { recovery_codes: codes } }))
    await page.getByRole('button', { name: /^(Enable|Bật)$/ }).click()
    await expect(page.locator('.auth-secret')).toBeVisible()
    if (state === 'recovery') {
      await page.locator('.account-security input[autocomplete="one-time-code"]').fill('123456')
      await page.getByRole('button', { name: /^(Confirm authenticator|Xác nhận ứng dụng xác thực)$/ }).click()
      await expect(page.locator('.recovery-codes')).toBeVisible()
    }
  }
  if (state === 'remove') await page.getByRole('button', { name: /^(Disable|Tắt)$/ }).click()
  if (state === 'change password') await page.getByRole('button', { name: /^(Change password|Đổi mật khẩu)$/ }).click()
  if (state === 'add notice') {
    await page.route('**/api/v1/me/password/link', route => route.fulfill({ json: {} }))
    await page.getByRole('button', { name: /^(Add a password by email|Thêm mật khẩu qua email)$/ }).click()
    await expect(page.locator('.account-security-notice[role="status"]')).toBeVisible()
  }
  if (state === 'token dialog' || state === 'token value') {
    await page.getByRole('button', { name: /^(Create token|Tạo mã mới)$/ }).click()
    await expect(page.locator('dialog[open]')).toBeVisible()
    if (state === 'token value') {
      await page.locator('dialog input').first().fill('fixture')
      await page.locator('dialog button[type="submit"]').click()
      await expect(page.locator('.token-value')).toBeVisible()
    }
  }
  if (state === 'delete dialog') await page.getByRole('button', { name: /^(Delete account…|Xóa tài khoản…)$/ }).click()
}

const states = [
  { name: '/', path: '/', kind: 'public' },
  { name: '/vn/topics', path: '/vn/topics', kind: 'public' },
  { name: '/vn/topics/1', path: '/vn/topics/1', kind: 'public' },
  { name: '/sources', path: '/sources', kind: 'public' },
  { name: '/connect', path: '/connect', kind: 'page' },
  { name: '/privacy', path: '/privacy', kind: 'page' },
  { name: '/terms', path: '/terms', kind: 'page' },
  { name: '/login', path: '/login', kind: 'page' },
  { name: '/login/factor', path: '/login/factor', kind: 'factor' },
  { name: '/signup', path: '/signup', kind: 'page' },
  { name: '/reset', path: '/reset', kind: 'page' },
  { name: '/link registration', path: '/link', kind: 'registration' },
  { name: '/link reset', path: '/link', kind: 'reset' },
  { name: '/link add_password', path: '/link', kind: 'add_password' },
  { name: '/vn/search hits', path: '/vn/search', kind: 'search' },
  { name: '/vn/documents/vn:vbpl:1', path: '/vn/documents/vn:vbpl:1', kind: 'reader' },
  ...['set', 'unset', 'setup', 'recovery', 'remove', 'change password', 'add notice', 'token dialog', 'token value', 'delete dialog'].map(name => ({ name: `/account ${name}`, path: name, kind: 'account' })),
]

for (const width of [320, 390, 768, 1280]) for (const language of ['vi', 'en']) for (const state of states) {
  test(`${state.name} ${width}px ${language}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 })
    await page.setExtraHTTPHeaders({ 'X-Forwarded-For': clientAddress(testInfo) })
    await page.context().addCookies([{ name: 'lang', value: language, url: 'http://127.0.0.1:3031' }])
    const visit = async (origin = 'http://127.0.0.1:3031') => {
      if (state.kind === 'public') {
        await page.context().addCookies([{ name: 'lang', value: language, url: origin }])
        const response = await page.goto(origin + state.path)
        expect(response?.status()).toBe(200)
        if (state.path === '/' || state.path === '/sources') await expect(page.locator('.coverage-stats dd')).toHaveCount(4)
        else await expect(page.locator('main').getByText('Fixture topic').first()).toBeVisible()
      } else if (state.kind === 'account') await accountState(page, state.path, language)
      else if (state.kind === 'search' || state.kind === 'reader') {
        await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
        await page.route('**/api/v1/search', route => route.fulfill({ json: { release: '20990101', abstain: false, hits: [{ citation: 'Điều 1', complete: true, text: 'Nội dung giả lập.' }] } }))
        await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', item: { complete: true, document: { number: '1/2099', title: 'Test law', status: { class: 'in_force', current: true }, text_source: 'none', text_complete: false }, units: [{ kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', text: 'Text' }] } } }))
        await page.goto('/')
        await expect(page.getByRole('link', { name: /account|tài khoản/i }).first()).toBeAttached()
        await navigate(page, state.path)
        if (state.kind === 'search') {
          await page.locator('#search-query').fill('Nội dung giả lập')
          await page.locator('.search-box').evaluate(form => (form as HTMLFormElement).requestSubmit())
          await expect(page.locator('.result-card .law-text')).toBeVisible()
        } else await expect(page.locator('.reader-identity .num')).toBeVisible()
      } else {
        await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
        await page.route('**/api/v1/auth/factor', route => route.fulfill({ json: { csrf: 'csrf', next: '/account' } }))
        await page.route('**/api/v1/auth/link', route => route.fulfill({ json: { purpose: state.kind, email: account.email } }))
        await page.goto(origin + state.path + (['registration', 'reset', 'add_password'].includes(state.kind) && state.path === '/link' ? '#synthetic-link-token' : ''))
        if (state.path === '/link') await expect(page.locator('input[name="new-password"]')).toBeVisible()
        if (state.kind === 'factor') await expect(page.locator('input[autocomplete="one-time-code"]')).toBeVisible()
      }
      await expect(page.locator('html')).toHaveAttribute('lang', language)
      await page.evaluate(() => document.fonts.ready)
      const offenders = await page.evaluate(({ allowlist }) => {
        const found: { selector: string, text: string, reason: string }[] = []
        const path = (element: Element): string => {
          const parts: string[] = []
          for (let el: Element | null = element; el; el = el.parentElement) {
            const siblings = el.parentElement ? Array.from(el.parentElement.children).filter(child => child.tagName === el!.tagName) : []
            parts.unshift(`${el.tagName.toLowerCase()}${el.id ? `#${CSS.escape(el.id)}` : ''}${siblings.length > 1 ? `:nth-of-type(${siblings.indexOf(el) + 1})` : ''}`)
          }
          return parts.join(' > ')
        }
        const add = (element: Element, reason: string) => found.push({ selector: path(element), text: (element.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 100), reason })
        if (document.documentElement.scrollWidth > innerWidth) add(document.documentElement, `page width ${document.documentElement.scrollWidth} > ${innerWidth}`)
        for (const element of document.querySelectorAll<HTMLElement>('*')) {
          const style = getComputedStyle(element)
          if (!element.getClientRects().length || style.visibility !== 'visible' || style.display === 'none') continue
          if (!allowlist.some(entry => element.matches(entry.selector)) && element.scrollWidth > element.clientWidth + 1 && style.overflowX !== 'visible') add(element, `horizontal overflow: ${style.overflowX}`)
          if (!allowlist.some(entry => element.matches(entry.selector)) && style.textOverflow === 'ellipsis' && element.scrollWidth > element.clientWidth + 1) add(element, 'text ellipsis')
        }
        const selectors = '.coverage-stats dd, .recovery-codes li span, .auth-secret, .num, .account-heading p span, .reader-meta strong, .token-value, [data-value], .connect-tools code, .page-code pre, .home-address code, .home-connect-card pre'
        for (const element of document.querySelectorAll<HTMLElement>(selectors)) {
          if (!element.getClientRects().length || getComputedStyle(element).visibility !== 'visible') continue
          const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT)
          while (walker.nextNode()) {
            const text = walker.currentNode as Text
            if (element.matches('[data-value=account-created]') && text.parentElement !== element) continue
            const split = element.matches('.recovery-codes li span, .auth-secret, .page-code pre, .home-connect-card pre')
            const startDate = element.matches('[data-value=account-created]') ? text.data.search(/\d/) : 0
            const chunks = split ? [...text.data.matchAll(/\S+/g)].map(match => [match.index, match.index + match[0].length]) : [[startDate, text.length]]
            for (const [start, end] of chunks) {
              if (start === undefined || end === undefined || start < 0 || start === end) continue
              const range = document.createRange()
              range.setStart(text, start); range.setEnd(text, end)
              const tops = Array.from(range.getClientRects(), rect => rect.top)
              if (tops.length && Math.max(...tops) - Math.min(...tops) > 2) add(element, 'value breaks within a segment')
            }
          }
        }
        return found
      }, { allowlist })
      if (offenders.length) {
        await testInfo.attach('responsive-offenders', { body: JSON.stringify(offenders, null, 2), contentType: 'application/json' })
        await testInfo.attach('responsive-screenshot', { body: await page.screenshot({ fullPage: true }), contentType: 'image/png' })
      }
      expect(offenders).toEqual([])
    }
    if (state.kind === 'public') await publicSite(visit)
    else await visit()
  })
}
