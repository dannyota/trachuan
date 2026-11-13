import { expect, test } from '@playwright/test'

const lead = 'Điều kiện giả lập cho các khoản.'
const first = 'Khoản đầu chứa nội dung giả lập dài. '.repeat(40)
const second = 'Khoản hai có dấu hiệu hợp nhất màu ngọc.'
const third = 'Khoản ba chỉ có trong bài đọc đầy đủ.'
const text = `${lead}\n${first}\n${second}`
const release = '20990101_3'
const doc = 'vn:fixture:siblings'

// The API fixture represents a recipe-3 hit with two complete constituents.
test('renders merged text and snippet and opens the article reader', async ({ page }) => {
  let searches = 0
  await page.route('**/api/v1/me', route => route.fulfill({ json: { id: 1, email: 'test@example.invalid', created_at: '2099-01-01T00:00:00Z', csrf_token: 'token' } }))
  await page.route('**/api/v1/search', route => {
    searches++
    const hit = { handle: `${doc}#article-1`, doc, citation: 'Điều 1', complete: false, nodes: [] }
    return route.fulfill({ json: {
      release, abstain: false, nodes: {},
      documents: { [doc]: { number: '29/2099/TT-TEST', title: 'Văn bản hợp nhất giả lập', status: { class: 'in_force', current: true }, text_source: 'source_tree', text_complete: true } },
      hits: [{ ...hit, ...(searches === 1 ? { snippet: `${lead} […] ${second}` } : { text, next: 'article-read', fragments: ['clause-1', 'clause-2'].map(anchor => ({ identity: { country: 'vn', document_key: doc, anchor }, complete: true })) }) }],
    } })
  })
  await page.route('**/api/v1/documents/*', route => {
    expect(new URL(route.request().url()).searchParams.get('release')).toBe(release)
    return route.fulfill({ json: { release, item: { complete: true, units: [{ kind: 'provision', anchor: 'article-1', text: lead }, { kind: 'provision', anchor: 'clause-1', text: first }, { kind: 'provision', anchor: 'clause-2', text: second }, { kind: 'provision', anchor: 'clause-3', text: third }] } } })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: /account|tài khoản/i })).toBeVisible()
  await page.evaluate(() => (document.querySelector('#__nuxt') as unknown as { __vue_app__: { config: { globalProperties: { $router: { push: (path: string) => Promise<void> } } } } }).__vue_app__.config.globalProperties.$router.push('/vn/search'))
  await page.locator('#search-query').fill(second)
  const submit = () => page.locator('form').evaluate(form => (form as HTMLFormElement).requestSubmit())
  await submit()
  await expect(page.locator('.law-text')).toHaveText(`${lead} […] ${second}`)
  await submit()
  await expect(page.locator('.law-text')).toHaveText(text)
  await expect(page.locator('.search-card')).toHaveCount(1)
  const reader = page.getByRole('link', { name: /read document|đọc văn bản/i })
  await expect(reader).toHaveAttribute('href', new RegExp(`release=${release}#article-1$`))
  await reader.click()
  await expect(page.getByText(third, { exact: true })).toBeVisible()
})
