import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createInterface } from 'node:readline'
import { expect, test, type Page } from '@playwright/test'
import { openAccount } from './account-fixture'
import { clientAddress } from './client-address'

const allowlist: { selector: string, reason: string }[] = [
  { selector: 'input.token-value[readonly]', reason: 'copy-only secret, single line, scrolls inside a readonly input' },
  { selector: '.login-card input#username[readonly]', reason: 'readonly email, scrolls only when longer than the field' },
]
const status = { release: '20990101', documents: 1234567, provisions: 9876543, nodes: 2, newest_source_date: '2099-12-28', gaps: {} }
const account = { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'csrf', password: true, google: false, totp: false, totp_enrollment: true }
const node = { code: '1', name: 'Fixture topic', provisions: 4, documents: 2 }
const codes = Array(10).fill('ABCDEFGHIJKLMNOPQRSTUVWXYZ')
const fileHash = 'abcdef012345'.repeat(5) + 'abcd'
const files = [
  { name: 'main.pdf', url: 'https://example.test/main.pdf', hash: fileHash, size: 1500, role: 'main', fetched_at: '2099-01-01T00:00:00Z' },
  { name: 'long-appendix_name.with-many-parts-for-the-reader.pdf', url: 'https://example.test/appendix', hash: fileHash, size: 2000000, role: 'appendix', fetched_at: '2099-01-02T00:00:00Z' },
  { name: 'scan.pdf', url: null, hash: fileHash, size: 3000, role: 'scan', fetched_at: null },
]
const longEmail = `${'a'.repeat(40)}@example.invalid`

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
  const email = state === 'long email' ? longEmail : account.email
  await page.route('**/api/v1/me', route => route.fulfill({ json: { ...account, email, password, google: !password, totp } }))
  await page.context().addCookies([
    { name: 'lang', value: language, url: 'http://127.0.0.1:3031' },
    { name: 'account-security', value: `${Number(password)}${Number(totp)}1`, url: 'http://127.0.0.1:3031' },
  ])
  await page.reload()
  await expect(page.locator('.account-security')).toBeVisible()
  if (state === 'long email') {
    await page.evaluate(email => {
      const app = (document.querySelector('#__nuxt') as unknown as { __vue_app__: { $nuxt: { $pinia: { state: { value: { session: { account: { email: string } } } } } } } }).__vue_app__
      app.$nuxt.$pinia.state.value.session.account.email = email
    }, email)
    await expect(page.locator('.account-email')).toHaveText(email)
  }
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
      await expect(page.locator('input.token-value[readonly]')).toHaveValue('trp_fixture_secret_only_once')
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
  { name: '/vn/documents/vn:vbpl:1 files', path: '/vn/documents/vn:vbpl:1', kind: 'reader-files' },
  ...['set', 'unset', 'long email', 'setup', 'recovery', 'remove', 'change password', 'add notice', 'token dialog', 'token value', 'delete dialog'].map(name => ({ name: `/account ${name}`, path: name, kind: 'account' })),
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
      else if (state.kind === 'search' || state.kind === 'reader' || state.kind === 'reader-files') {
        await page.route('**/api/v1/me', route => route.fulfill({ json: account }))
        await page.route('**/api/v1/search', route => route.fulfill({ json: { release: '20990101', abstain: false, hits: [{ citation: 'Điều 1', complete: true, text: 'Nội dung giả lập.' }] } }))
        await page.route('**/api/v1/documents/*', route => route.fulfill({ json: { release: '20990101', fetch_sha256: fileHash, files: state.kind === 'reader-files' ? files : [], item: { complete: true, document: { number: '1/2099', title: 'Test law', status: { class: 'in_force', current: true }, text_source: 'none', text_complete: false }, units: [{ kind: 'provision', anchor: 'dieu-1', label: 'Điều 1', text: 'Text' }] } } }))
        await page.goto('/')
        const menu = page.locator('button[aria-controls="site-menu"]')
        if (await menu.isVisible()) await menu.click()
        await expect(page.getByRole('link', { name: /^(account|tài khoản)$/i })).toBeVisible()
        if (await menu.isVisible()) await menu.click()
        await navigate(page, state.path)
        if (state.kind === 'search') {
          await page.locator('#search-query').fill('Nội dung giả lập')
          await page.locator('.search-box').evaluate(form => (form as HTMLFormElement).requestSubmit())
          await expect(page.locator('.result-card .law-text')).toBeVisible()
        } else {
          await expect(page.locator('.reader-identity .num')).toBeVisible()
          await page.locator('.reader-rail').getByText(language === 'vi' ? 'Chi tiết kỹ thuật' : 'Technical details', { exact: true }).click()
          await expect(page.locator('.reader-technical')).toBeVisible()
          if (state.kind === 'reader-files') await expect(page.locator('.reader-file')).toHaveCount(3)
        }
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
          const clip = /^rect\(([-\d.]+)px,?\s+([-\d.]+)px,?\s+([-\d.]+)px,?\s+([-\d.]+)px\)$/.exec(style.clip)
          const clipped = clip && Number(clip[2]) - Number(clip[4]) <= 1 && Number(clip[3]) - Number(clip[1]) <= 1
          if (!element.getClientRects().length || style.visibility !== 'visible' || style.display === 'none' || element.classList.contains('sr-only') || clipped) continue
          if (element.matches('.login-card input#username[readonly]') && element.scrollWidth > element.clientWidth + 1) add(element, 'fixture email does not fit')
          if (!allowlist.some(entry => element.matches(entry.selector)) && element.scrollWidth > element.clientWidth + 1 && style.overflowX !== 'visible') add(element, `horizontal overflow: ${style.overflowX}`)
          if (!allowlist.some(entry => element.matches(entry.selector)) && style.textOverflow === 'ellipsis' && element.scrollWidth > element.clientWidth + 1) add(element, 'text ellipsis')
        }
        for (const block of document.querySelectorAll('.page-code pre, .home-connect-card pre')) {
          const selection = window.getSelection()
          const range = document.createRange()
          range.selectNodeContents(block)
          selection?.removeAllRanges()
          selection?.addRange(range)
          if (selection?.toString() !== block.textContent) add(block, 'selection differs from command text')
          selection?.removeAllRanges()
        }
        const selectors = '.coverage-stats dd, .recovery-codes li span, .auth-secret, .num, .account-email, .reader-meta strong, .token-value, [data-value], .connect-tools code, .page-code pre, .home-address code, .home-connect-card pre, .reader-file-name, .reader-file-value'
        for (const element of document.querySelectorAll<HTMLElement>(selectors)) {
          if (!element.getClientRects().length || getComputedStyle(element).visibility !== 'visible') continue
          const emailSegmentsBreakOnlyAfterAtOrDot = element.matches('.account-email')
          if (emailSegmentsBreakOnlyAfterAtOrDot) {
            for (const breakElement of element.querySelectorAll('wbr')) {
              if (!/[@.]$/.test(breakElement.previousElementSibling?.textContent || '')) add(element, 'email break is not after @ or .')
            }
          }
          const split = element.matches('.recovery-codes li span, .auth-secret, .connect-tools code, .page-code pre, .home-connect-card pre, .account-email, .reader-file-name')
          if (!split) {
            const range = document.createRange()
            range.selectNodeContents(element)
            const tops = Array.from(range.getClientRects(), rect => rect.top)
            if (tops.length && Math.max(...tops) - Math.min(...tops) > 2) add(element, 'value breaks within a segment')
            continue
          }
          const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT)
          while (walker.nextNode()) {
            const text = walker.currentNode as Text
            const chunks = [...text.data.matchAll(/\S+/g)].map(match => [match.index, match.index + match[0].length])
            for (const [start, end] of chunks) {
              if (start === undefined || end === undefined || start < 0 || start === end) continue
              const range = document.createRange()
              range.setStart(text, start); range.setEnd(text, end)
              const tops = Array.from(range.getClientRects(), rect => rect.top)
              if (tops.length && Math.max(...tops) - Math.min(...tops) > 2) {
                let exceedsLine = false
                if (element.matches('.page-code pre, .home-connect-card pre, .account-email, .reader-file-name')) {
                  const probe = document.createElement('span')
                  probe.textContent = text.data.slice(start, end)
                  probe.style.cssText = 'position:fixed;visibility:hidden;display:inline-block;white-space:nowrap;width:max-content;max-width:none;overflow-wrap:normal;word-break:normal'
                  const container = element.matches('.account-email') ? element.parentElement! : element
                  container.append(probe)
                  const style = getComputedStyle(container)
                  const contentWidth = container.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
                  exceedsLine = probe.getBoundingClientRect().width > contentWidth
                  probe.remove()
                }
                if (!exceedsLine) add(element, 'value breaks within a segment')
              }
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
